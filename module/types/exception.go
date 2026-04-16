package types

import "fmt"

type ServerException struct {
	Code    string
	Message string
}

func (this ServerException) Error() string {
	return "Server exception occured: " + this.Code + "\n" + this.Message
}

type InvalidNameOrKeyError struct {
	Name string
}

func (this InvalidNameOrKeyError) Error() string {
	return "Invalid name or key: " + this.Name
}

type InvalidHeaderError struct {
	Header map[string]string
}

func (this InvalidHeaderError) Error() string {
	return fmt.Sprintf("Invalid header: %v", this.Header)
}

type RegiesterMessageError struct {
	Raw string
}

func (this RegiesterMessageError) Error() string {
	return "Register message error occured: " + this.Raw
}
