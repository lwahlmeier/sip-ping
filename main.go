package main

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/binary"
	"flag"
	"math"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/PremiereGlobal/stim/pkg/stimlog"
	"github.com/gorilla/websocket"
)

const alphanum = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

var addr = flag.String("addr", "", "http service address")
var skipVerify = flag.Bool("skipverify", false, "skip TLS certificate verification")
var debug = flag.Bool("debug", false, "Do debug logging")
var log stimlog.StimLogger = stimlog.GetLogger()

const WS_OPTIONS = `OPTIONS sip:monitor@none SIP/2.0
Via: SIP/2.0/{{PROTOC}} 81okseq92jb7.invalid;branch=z9hG4bK5964427
To: <sip:ba_user@none>
From: <sip:anonymous.8scs48@anonymous.invalid>;tag=fql2c8mlg3
Call-ID: {{callId}}
CSeq: {{seq}} OPTIONS
Content-Length: 0

` // two newlines required to signal end of request

const TCP_OPTIONS = `OPTIONS sip:host@invalid:1739;transport={{proto}} SIP/2.0
Via: SIP/2.0/{{PROTOC}} {{localaddr}};branch=z9hG4bKr1t13cmvZDjtg
Max-Forwards: 70
From: "" <sip:monitor@invalid>
To: <sip:host@invalid;transport={{proto}}>
Call-ID: {{callId}}
CSeq: {{seq}} OPTIONS
Content-Length: 0

` // two newlines required to signal end of request

func randString(n int) string {
	var bytes = make([]byte, n)
	rand.Read(bytes)
	for i, b := range bytes {
		bytes[i] = alphanum[b%byte(len(alphanum))]
	}
	return string(bytes)
}

func renderRequest(options string, la string, protocol string) string {
	var bytes = make([]byte, 2)
	rand.Read(bytes)
	seq := binary.BigEndian.Uint16(bytes)
	req := strings.Replace(options, "{{callId}}", randString(20), -1)
	req = strings.Replace(req, "{{localaddr}}", la, -1)
	req = strings.Replace(req, "{{PROTOC}}", strings.ToUpper(protocol), -1)
	req = strings.Replace(req, "{{proto}}", strings.ToLower(protocol), -1)
	req = strings.Replace(req, "{{seq}}", strconv.Itoa(int(seq)), -1)
	req = strings.Replace(req, "\n", "\r\n", -1)
	return req
}

type sipResponse struct {
	response      string
	response_time time.Duration
}

func main() {
	flag.Parse()
	if *debug {
		log.SetLevel(stimlog.DebugLevel)
		log.Debug("Debug logging enabled")
	} else {
		log.SetLevel(stimlog.InfoLevel)
	}

	log.ForceFlush(true)
	interrupt := make(chan os.Signal, 1)
	log.Debug("Setting up interrupt signal handler")
	signal.Notify(interrupt, os.Interrupt)

	if len(*addr) == 0 {
		log.Warn("No addr paramiter found!")
		flag.Usage()
		os.Exit(1)
	}

	var url, err = url.Parse(*addr)
	if err != nil {
		log.Fatal("addr:", err)
	}
	log.Debug("Got addr: \"{}\"", url)

	response := make(chan sipResponse)
	request := make(chan string)
	response_err := make(chan error)

	if url.Scheme == "wss" || url.Scheme == "ws" {
		log.Debug("Doing websocket sip check")
		go queryWS(url, request, response, response_err)
	} else if url.Scheme == "udp" || url.Scheme == "tcp" || url.Scheme == "tls" {
		log.Debug("Doing normal sip check")
		go querySIP(url, skipVerify, request, response, response_err)
	} else {
		log.Fatal("Unknown scheme:", url.Scheme)
	}
	for {
		select {
		case <-time.After(15 * time.Second):
			log.Fatal("Timed out waiting for response")
		case err := <-response_err:
			log.Fatal("Got Error response, {}", err)
		case <-interrupt:
			log.Fatal("Interrupted")
		case req := <-request:
			log.Info("Request:\n\t{}", strings.ReplaceAll(req, "\r\n", "\n\t"))
		case resp := <-response:
			log.Info("Response({}ms):\n\t{}", math.Round(resp.response_time.Seconds()*100000)/100, strings.ReplaceAll(resp.response, "\r\n", "\n\t"))
			if strings.Contains(resp.response, "SIP/2.0 200 OK") {
				os.Exit(0)
			} else {
				os.Exit(1)
			}
		}
	}
}

func querySIP(url *url.URL, skipVerify *bool, request chan string, response chan sipResponse, response_err chan error) {
	var err error
	var conn net.Conn
	if url.Scheme == "tls" {
		log.Debug("doing TLS sip")
		tlsClientConfig := &tls.Config{InsecureSkipVerify: *skipVerify}
		conn, err = tls.Dial("tcp", url.Host, tlsClientConfig)
	} else if url.Scheme == "udp" {
		log.Debug("Doing UDP sip")
		conn, err = net.Dial("udp", url.Host)
	} else {
		log.Debug("Doing TCP sip")
		conn, err = net.DialTimeout("tcp", url.Host, time.Second*5)
	}
	if err != nil {
		response_err <- err
		return
	}
	defer conn.Close()
	log.Debug("Rendering SIP Request")
	req := renderRequest(TCP_OPTIONS, conn.LocalAddr().String(), url.Scheme)
	log.Debug("Request Rendered, writting request")
	_, err = conn.Write([]byte(req))
	if err != nil {
		response_err <- err
		return
	}
	start_time := time.Now()
	request <- req
	log.Debug("Wrote request")
	buf := make([]byte, 0, 65536)
	tmp := make([]byte, 4096)
	for {
		n, err := conn.Read(tmp)
		if err != nil {
			response_err <- err
			return
		}
		buf = append(buf, tmp[:n]...)
		s := string(buf)
		if strings.Contains(s, "\r\n\r\n") || len(buf) >= 65536 {
			response <- sipResponse{string(buf), time.Since(start_time)}
			return
		}
	}
}

func queryWS(url *url.URL, request chan string, response chan sipResponse, response_err chan error) {
	var tlsClientConfig = &tls.Config{InsecureSkipVerify: *skipVerify}
	var sipDialer = websocket.Dialer{
		Subprotocols:    []string{"sip"},
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		TLSClientConfig: tlsClientConfig,
	}

	c, _, err := sipDialer.Dial(*addr, nil)
	if err != nil {
		response_err <- err
		return
	}

	defer c.Close()

	req := renderRequest(WS_OPTIONS, "", url.Scheme)

	err = c.WriteMessage(websocket.TextMessage, []byte(req))
	if err != nil {
		response_err <- err
		return
	}
	start_time := time.Now()
	request <- req
	_, message, err := c.ReadMessage()
	if err != nil {
		response_err <- err
		return
	}
	response <- sipResponse{strings.ReplaceAll(string(message), "\\r\\n", "\r\n"), time.Since(start_time)}
}
