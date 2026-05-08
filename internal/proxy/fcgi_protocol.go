package proxy

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
)

// FastCGI record types
const (
	fcgiBeginRequest    = 1
	fcgiAbortRequest    = 2
	fcgiEndRequest      = 3
	fcgiParams          = 4
	fcgiStdin           = 5
	fcgiStdout          = 6
	fcgiStderr          = 7
	fcgiData            = 8
	fcgiGetValues       = 9
	fcgiGetValuesResult = 10
	fcgiUnknownType     = 11

	fcgiResponder = 1
	fcgiKeepConn  = 1

	fcgiRequestComplete = 0
	fcgiMaxContentLen   = 65535
)

// fcgiHeader is the FastCGI record header
type fcgiHeader struct {
	Version       uint8
	Type          uint8
	RequestIDB1   uint8
	RequestIDB0   uint8
	ContentLenB1  uint8
	ContentLenB0  uint8
	PaddingLength uint8
	Reserved      uint8
}

// fcgiRequest represents a FastCGI request to be sent to php-cgi
type fcgiRequest struct {
	params map[string]string
	body   io.Reader
}

func newFCGIRequest(r *http.Request, scriptFilename, scriptName, documentRoot string) *fcgiRequest {
	params := map[string]string{
		"REQUEST_METHOD":    r.Method,
		"SCRIPT_FILENAME":   scriptFilename,
		"SCRIPT_NAME":       scriptName,
		"REQUEST_URI":       r.RequestURI,
		"DOCUMENT_ROOT":     documentRoot,
		"SERVER_SOFTWARE":   "GopherStack/1.0",
		"REMOTE_ADDR":       extractIP(r.RemoteAddr),
		"REMOTE_PORT":       extractPort(r.RemoteAddr),
		"SERVER_ADDR":       "127.0.0.1",
		"SERVER_PORT":       "80",
		"SERVER_NAME":       r.Host,
		"SERVER_PROTOCOL":   r.Proto,
		"GATEWAY_INTERFACE": "CGI/1.1",
		"QUERY_STRING":      r.URL.RawQuery,
		"REDIRECT_STATUS":   "200",
	}

	// Add content type and length for POST requests
	if r.ContentLength > 0 {
		params["CONTENT_LENGTH"] = strconv.FormatInt(r.ContentLength, 10)
	}
	if ct := r.Header.Get("Content-Type"); ct != "" {
		params["CONTENT_TYPE"] = ct
	}

	// Add HTTP headers as params
	for key, vals := range r.Header {
		headerKey := "HTTP_" + strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
		if headerKey == "HTTP_CONTENT_TYPE" || headerKey == "HTTP_CONTENT_LENGTH" {
			continue // Already handled above
		}
		params[headerKey] = strings.Join(vals, ", ")
	}

	return &fcgiRequest{
		params: params,
		body:   r.Body,
	}
}

func (req *fcgiRequest) writeTo(conn net.Conn) error {
	requestID := uint16(1)

	// Send FCGI_BEGIN_REQUEST
	beginBody := make([]byte, 8)
	binary.BigEndian.PutUint16(beginBody[:2], fcgiResponder)
	beginBody[2] = 0 // flags (no keep-conn)
	if err := writeRecord(conn, fcgiBeginRequest, requestID, beginBody); err != nil {
		return fmt.Errorf("failed to write begin request: %w", err)
	}

	// Send FCGI_PARAMS
	paramsData := encodeParams(req.params)
	if err := writeRecord(conn, fcgiParams, requestID, paramsData); err != nil {
		return fmt.Errorf("failed to write params: %w", err)
	}

	// Send empty FCGI_PARAMS to signal end
	if err := writeRecord(conn, fcgiParams, requestID, nil); err != nil {
		return fmt.Errorf("failed to write empty params: %w", err)
	}

	// Send FCGI_STDIN (request body)
	if req.body != nil {
		buf := make([]byte, fcgiMaxContentLen)
		for {
			n, err := req.body.Read(buf)
			if n > 0 {
				if writeErr := writeRecord(conn, fcgiStdin, requestID, buf[:n]); writeErr != nil {
					return fmt.Errorf("failed to write stdin: %w", writeErr)
				}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("failed to read request body: %w", err)
			}
		}
	}

	// Send empty FCGI_STDIN to signal end
	if err := writeRecord(conn, fcgiStdin, requestID, nil); err != nil {
		return fmt.Errorf("failed to write empty stdin: %w", err)
	}

	return nil
}

func readFCGIResponse(conn net.Conn, w http.ResponseWriter) error {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	reader := bufio.NewReader(conn)

	for {
		header := fcgiHeader{}
		if err := binary.Read(reader, binary.BigEndian, &header); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("failed to read record header: %w", err)
		}

		contentLen := int(header.ContentLenB1)<<8 | int(header.ContentLenB0)
		paddingLen := int(header.PaddingLength)

		content := make([]byte, contentLen)
		if contentLen > 0 {
			if _, err := io.ReadFull(reader, content); err != nil {
				return fmt.Errorf("failed to read record content: %w", err)
			}
		}

		// Discard padding
		if paddingLen > 0 {
			padding := make([]byte, paddingLen)
			io.ReadFull(reader, padding)
		}

		switch header.Type {
		case fcgiStdout:
			stdout.Write(content)
		case fcgiStderr:
			stderr.Write(content)
		case fcgiEndRequest:
			goto done
		}
	}

done:
	// Parse the HTTP response from stdout
	if stdout.Len() == 0 {
		if stderr.Len() > 0 {
			http.Error(w, "PHP Error: "+stderr.String(), http.StatusInternalServerError)
		} else {
			http.Error(w, "Empty response from PHP", http.StatusBadGateway)
		}
		return nil
	}

	return parseCGIResponse(&stdout, w)
}

func parseCGIResponse(stdout *bytes.Buffer, w http.ResponseWriter) error {
	reader := bufio.NewReader(stdout)

	statusCode := http.StatusOK

	// Read headers from CGI output
	for {
		line, err := reader.ReadString('\n')
		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			break // End of headers
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			if err == io.EOF {
				break
			}
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if strings.ToLower(key) == "status" {
			// Parse status code from "Status: 200 OK"
			statusParts := strings.SplitN(value, " ", 2)
			if code, parseErr := strconv.Atoi(statusParts[0]); parseErr == nil {
				statusCode = code
			}
			continue
		}

		w.Header().Set(key, value)

		if err == io.EOF {
			break
		}
	}

	// Write status code and body
	w.WriteHeader(statusCode)
	io.Copy(w, reader)

	return nil
}

// writeRecord writes a FastCGI record
func writeRecord(conn net.Conn, recType uint8, requestID uint16, content []byte) error {
	contentLen := len(content)
	paddingLen := (8 - contentLen%8) % 8

	header := fcgiHeader{
		Version:       1,
		Type:          recType,
		RequestIDB1:   uint8(requestID >> 8),
		RequestIDB0:   uint8(requestID),
		ContentLenB1:  uint8(contentLen >> 8),
		ContentLenB0:  uint8(contentLen),
		PaddingLength: uint8(paddingLen),
	}

	if err := binary.Write(conn, binary.BigEndian, &header); err != nil {
		return err
	}

	if contentLen > 0 {
		if _, err := conn.Write(content); err != nil {
			return err
		}
	}

	if paddingLen > 0 {
		padding := make([]byte, paddingLen)
		if _, err := conn.Write(padding); err != nil {
			return err
		}
	}

	return nil
}

// encodeParams encodes FastCGI params as name-value pairs
func encodeParams(params map[string]string) []byte {
	var buf bytes.Buffer

	for name, value := range params {
		nameLen := len(name)
		valueLen := len(value)

		// Encode name length
		if nameLen < 128 {
			buf.WriteByte(byte(nameLen))
		} else {
			buf.WriteByte(byte(nameLen>>24) | 0x80)
			buf.WriteByte(byte(nameLen >> 16))
			buf.WriteByte(byte(nameLen >> 8))
			buf.WriteByte(byte(nameLen))
		}

		// Encode value length
		if valueLen < 128 {
			buf.WriteByte(byte(valueLen))
		} else {
			buf.WriteByte(byte(valueLen>>24) | 0x80)
			buf.WriteByte(byte(valueLen >> 16))
			buf.WriteByte(byte(valueLen >> 8))
			buf.WriteByte(byte(valueLen))
		}

		buf.WriteString(name)
		buf.WriteString(value)
	}

	return buf.Bytes()
}

func extractIP(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

func extractPort(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "0"
	}
	return port
}
