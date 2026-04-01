package server

import (
	"bufio"
	"encoding/json"
	"log"
	"net"
	"sync"
)

type Server struct {
	Listener         net.Listener
	Client           map[string]net.Conn
	ClientWriteMutex map[string]*sync.Mutex
	ClientMapMutex   *sync.Mutex
	Listening        bool
	KeyChecker       KeyChecker
}

type ClientCheckData struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

func CreateServer(port string, keyChecker KeyChecker) (*Server, error) {
	listener, err := net.Listen("tcp", port)
	if err != nil {
		return nil, err
	}

	server := &Server{
		Listener:         listener,
		Client:           make(map[string]net.Conn),
		ClientWriteMutex: make(map[string]*sync.Mutex),
		ClientMapMutex:   &sync.Mutex{},
		Listening:        false,
		KeyChecker:       keyChecker,
	}

	return server, nil
}

func (this *Server) Listen() {
	this.Listening = true
	go func() {
		for {
			conn, err := this.Listener.Accept()
			if err != nil {
				log.Println("Error accepting:", err)
				continue
			}
			go this.handleRequest(conn)
		}
	}()
}

func (this *Server) handleRequest(conn net.Conn) {
	from := ""
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[CRITICAL] Panic recovered in handleRequest for %s: %v", from, r)
		}
	}()
	defer func() {
		this.ClientMapMutex.Lock()
		delete(this.Client, from)
		delete(this.ClientWriteMutex, from)
		conn.Close()
		this.ClientMapMutex.Unlock()
	}()
	reader := bufio.NewReader(conn)

	// check client
	from, err := this.checkClient(reader)
	if err != nil {
		log.Println("[Error] Error checking client\n", err)
		return
	}
	registerSuccess := this.registerClient(conn, from)
	if !registerSuccess {
		log.Println("[Error]", from, "already exists.")
		return
	}

	// read
	for {
		alive := this.passMessage(reader, from)
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

	clientCheckData := ClientCheckData{}
	err = json.Unmarshal([]byte(line), &clientCheckData)
	if err != nil {
		return "", err
	}

	check := this.KeyChecker.Check(clientCheckData.Name, clientCheckData.Key)
	if !check {
		return "", &ServerException{code: "INVALID_NAME_OR_KEY"}
	}

	return clientCheckData.Name, nil
}

func (this *Server) registerClient(conn net.Conn, name string) (success bool) {
	this.ClientMapMutex.Lock()
	defer this.ClientMapMutex.Unlock()

	if _, exists := this.Client[name]; exists {
		return false
	}
	this.Client[name] = conn
	this.ClientWriteMutex[name] = &sync.Mutex{}
	return true
}

/*
이 함수에서, read에 실패했다면 sender와 연결을 종료하기 위해 false를 반환한다.
write에 실패했다면 추가적인 write만 실행하지 않고 계속한다.
*/
func (this *Server) passMessage(reader *bufio.Reader, from string) (alive bool) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[CRITICAL] Panic recovered in handleRequest for %s: %v", from, r)
		}
	}()

	header, err := this.readHeader(reader)
	if err != nil {
		log.Println("[Error] Reading header from", from, ":\n", err)
		return false
	}

	noDestination := false
	destination := header["destination"]
	writeMutex := this.ClientWriteMutex[destination]
	if writeMutex == nil {
		noDestination = true
	} else {
		writeMutex.Lock()
		defer writeMutex.Unlock()
	}
	writer := this.getDestinationWriter(destination)
	if writer == nil {
		noDestination = true
	}
	if noDestination {
		log.Printf("[Error] No destination: from '%s' to '%s'\n", from, destination)
	}

	success := this.sendHeader(header, from, writer)
	if !success {
		writer = nil
	}

	alive, success = this.passBody(reader, writer)
	if alive && success {
		log.Printf("[Success] Message passed from '%s' to '%s'\n", from, destination)
	} else if !alive {
		log.Printf("[Error] Error reading stream from '%s'\n", from)
	} else if !success && !noDestination {
		log.Printf("[Error] Error sending message from '%s' to '%s'\n", from, destination)
	}
	return alive
}

func (this *Server) readHeader(reader *bufio.Reader) (map[string]string, error) {
	headerJSON, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	headerJSON = headerJSON[:len(headerJSON)-1]

	header := make(map[string]string)
	err = json.Unmarshal([]byte(headerJSON), &header)
	if err != nil {
		return nil, err
	}

	if header["destination"] != "" && header["id"] != "" {
		return header, nil
	} else {
		return nil, &ServerException{code: "NO_DESTINATION_OR_ID_IN_HEADER"}
	}
}

func (this *Server) getDestinationWriter(destinationName string) *bufio.Writer {
	this.ClientMapMutex.Lock()
	defer this.ClientMapMutex.Unlock()
	destination := this.Client[destinationName]
	if destination != nil {
		return bufio.NewWriter(destination)
	}
	return nil
}

func (this *Server) sendHeader(header map[string]string, from string, destinationWriter *bufio.Writer) (success bool) {
	if destinationWriter == nil {
		return false
	}

	header["from"] = from
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return false
	}

	_, err = destinationWriter.WriteString(string(headerJSON) + "\n")
	if err != nil {
		return false
	}

	destinationWriter.Flush()
	return true
}

func (this *Server) passBody(reader *bufio.Reader, writer *bufio.Writer) (alive bool, success bool) {
	endFlag := 0
	for {
		b, err := reader.ReadByte()
		if err != nil {
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
