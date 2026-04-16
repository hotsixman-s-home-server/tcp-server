package server

import (
	"NDJFlow/module/types"
	"bufio"
	"encoding/json"
	"log"
	"net"
	"os"
	"strings"
	"sync"
)

type Server struct {
	TCPListener    net.Listener
	UDSListener    net.Listener
	Client         map[string]*Client
	ClientMapMutex *sync.Mutex
	Listening      bool
	KeyChecker     KeyChecker
}

type RegisterMessage struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

type Client struct {
	Name   string
	Conn   net.Conn
	Mutex  *sync.Mutex
	Reader *bufio.Reader
	Writer *bufio.Writer
}

func CreateServer(port string, udsPath string, keyChecker KeyChecker) (*Server, error) {
	var TCPListener net.Listener = nil
	var UDSListener net.Listener = nil
	var err error

	if port != "" {
		TCPListener, err = net.Listen("tcp", port)
		if err != nil {
			return nil, err
		}
	}
	if udsPath != "" {
		if err := os.Remove(udsPath); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		UDSListener, err = net.Listen("unix", udsPath)
		if err != nil {
			return nil, err
		}
	}

	server := &Server{
		TCPListener:    TCPListener,
		UDSListener:    UDSListener,
		Client:         make(map[string]*Client),
		ClientMapMutex: &sync.Mutex{},
		Listening:      false,
		KeyChecker:     keyChecker,
	}

	return server, nil
}

func (this *Server) Listen() {
	this.Listening = true
	go func() {
		if this.TCPListener != nil {
			for {
				conn, err := this.TCPListener.Accept()
				if err != nil {
					//log.Println("Error accepting:", err)
					continue
				}
				go this.handleRequest(conn)
			}
		}
	}()
	go func() {
		if this.UDSListener != nil {
			for {
				conn, err := this.UDSListener.Accept()
				if err != nil {
					//log.Println("Error accepting:", err)
					continue
				}
				go this.handleRequest(conn)
			}
		}
	}()
}

func (this *Server) handleRequest(conn net.Conn) {
	name := ""
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[CRITICAL] Panic recovered in handleRequest for %s: %v", name, r)
		}
	}()
	defer func() {
		this.ClientMapMutex.Lock()
		delete(this.Client, name)
		conn.Close()
		this.ClientMapMutex.Unlock()
	}()
	reader := bufio.NewReader(conn)

	// check client
	name, err := this.checkClient(reader)
	if err != nil {
		log.Println("[Error] Error checking client\n", err)
		return
	}
	registerSuccess, from := this.registerClient(conn, name, reader)
	if !registerSuccess {
		log.Println("[Error]", name, "already exists.")
		return
	}

	// read
	for {
		alive := this.passMessage(from)
		if !alive {
			return
		}
	}
}

func (this *Server) checkClient(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	registerMessage := RegisterMessage{}
	err = json.Unmarshal([]byte(line), &registerMessage)
	if err != nil {
		return "", err
	}

	check := this.KeyChecker.Check(registerMessage.Name, registerMessage.Key)
	if !check {
		return "", &types.InvalidNameOrKeyError{Name: registerMessage.Name}
	}

	return registerMessage.Name, nil
}

func (this *Server) registerClient(conn net.Conn, name string, reader *bufio.Reader) (success bool, client *Client) {
	this.ClientMapMutex.Lock()
	defer this.ClientMapMutex.Unlock()

	if _, exists := this.Client[name]; exists {
		return false, nil
	}

	client = &Client{
		Name:   name,
		Conn:   conn,
		Mutex:  &sync.Mutex{},
		Reader: reader,
		Writer: bufio.NewWriter(conn),
	}
	this.Client[name] = client
	return true, client
}

/*
이 함수에서, read에 실패했다면 sender와 연결을 종료하기 위해 false를 반환한다.
write에 실패했다면 추가적인 write만 실행하지 않고 계속한다.
*/
func (this *Server) passMessage(from *Client) (alive bool) {
	header, err := this.readHeader(from.Reader)
	if err != nil {
		log.Println("[Error] Reading header from", from, ":\n", err)
		return false
	}

	toName := header["to"]
	to := this.Client[header["to"]]
	if to == nil {
		log.Printf("[Error] No destination: from '%s' to '%s'\n", from.Name, header["to"])
	} else {
		to.Mutex.Lock()
		defer to.Mutex.Unlock()
	}

	success := this.sendHeader(header, from, to)

	alive, success = this.passBody(from, to)
	if alive && success {
		log.Printf("[Success] Message passed from '%s' to '%s'\n", from.Name, toName)
	} else if !alive {
		log.Printf("[Error] Error reading message from '%s'\n", from.Name)
	} else if !success {
		log.Printf("[Error] Error sending message from '%s' to '%s'\n", from.Name, toName)
		this.responseErrorFlag(header, from)
	}
	return alive
}

func (this *Server) readHeader(reader *bufio.Reader) (map[string]string, error) {
	headerJSON, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	headerJSON = strings.TrimSpace(headerJSON)

	header := make(map[string]string)
	err = json.Unmarshal([]byte(headerJSON), &header)
	if err != nil {
		return nil, err
	}

	if header["to"] != "" && header["id"] != "" {
		return header, nil
	} else {
		return nil, &types.InvalidHeaderError{Header: header}
	}
}

func (this *Server) sendHeader(header map[string]string, from *Client, to *Client) (success bool) {
	if to == nil {
		return false
	}

	header["from"] = from.Name
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return false
	}

	_, err = to.Writer.WriteString(string(headerJSON) + "\n")
	if err != nil {
		return false
	}

	to.Writer.Flush()
	return true
}

func (this *Server) passBody(from *Client, to *Client) (alive bool, success bool) {
	var writer *bufio.Writer = nil
	if to != nil {
		writer = to.Writer
	}
	endFlag := 0
	for {
		b, err := from.Reader.ReadByte()
		if err != nil {
			if writer != nil {
				writer.WriteString("\n1\n")
				writer.Flush()
			}
			return false, false
		}

		if b == 0 {
			endFlag++
		} else if b == '\n' && endFlag == 1 {
			endFlag++
		} else {
			endFlag = 0
		}

		if writer != nil {
			err := writer.WriteByte(b)
			if err != nil {
				writer = nil
			}
		}

		if endFlag == 2 {
			if writer != nil {
				writer.Flush()
			}
			break
		}
	}

	return true, writer != nil
}

func (this *Server) responseErrorFlag(header map[string]string, from *Client) {
	toName := header["to"]
	header["from"] = toName
	header["to"] = from.Name

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return
	}

	from.Mutex.Lock()
	defer from.Mutex.Unlock()
	_, err = from.Writer.WriteString(string(headerJSON) + "\n1\n")
	if err != nil {
		return
	}

	from.Writer.Flush()
}
