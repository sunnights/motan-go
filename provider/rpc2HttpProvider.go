package provider

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	motan "github.com/weibocom/motan-go/core"
)

// RPCWithHTTPProvider struct
type RPCWithHTTPProvider struct {
	url        *motan.URL
	port       int
	httpClient http.Client
	srvURLMap  srvURLMapT
	gctx       *motan.Context
}

func (h *RPCWithHTTPProvider) SetService(s interface{}) {
	fmt.Println()
}

func (h *RPCWithHTTPProvider) GetURL() *motan.URL {
	return h.url
}

func (h *RPCWithHTTPProvider) SetURL(url *motan.URL) {
	h.url = url
}

func (h *RPCWithHTTPProvider) IsAvailable() bool {
	return true
}

func (h *RPCWithHTTPProvider) GetPath() string {
	return h.url.Path
}

// Initialize http provider
func (h *RPCWithHTTPProvider) Initialize() {
	h.httpClient = http.Client{Timeout: 1 * time.Second}
	h.srvURLMap = make(srvURLMapT)
	h.port, _ = strconv.Atoi(h.gctx.AgentURL.GetParam("httpServerPort", "0"))
}

// Destroy a RPCWithHTTPProvider
func (h *RPCWithHTTPProvider) Destroy() {
}

// SetSerialization for set a motan.SetSerialization to RPCWithHTTPProvider
func (h *RPCWithHTTPProvider) SetSerialization(s motan.Serialization) {}

// SetProxy for RPCWithHTTPProvider
func (h *RPCWithHTTPProvider) SetProxy(proxy bool) {}

// SetContext use to set globle config to RPCWithHTTPProvider
func (h *RPCWithHTTPProvider) SetContext(context *motan.Context) {
	h.gctx = context
}

// Call for do a motan call through this provider
func (h *RPCWithHTTPProvider) Call(request motan.Request) motan.Response {
	// rpc to http
	var rs string
	var rb []byte
	reply := []interface{}{&rs, &rb}
	err := request.ProcessDeserializable(reply)
	if err != nil {
		return motan.BuildExceptionResponse(request.GetRequestID(), &motan.Exception{ErrCode: 500, ErrMsg: "deserialize arguments fail." + err.Error(), ErrType: motan.ServiceException})
	}
	httpRequest, _ := http.ReadRequest(bufio.NewReader(bytes.NewReader(rb)))
	u, _ := url.Parse(h.extractUrl(httpRequest))
	httpRequest.URL = u
	httpRequest.RequestURI = ""
	httpResponse, err := h.httpClient.Do(httpRequest)
	if err != nil {
		fmt.Println(err)
	}
	httpResponse.Header.Set("agent-server-address", h.url.GetAddressStr())
	rawResponseBytes, _ := httputil.DumpResponse(httpResponse, true)

	response := &motan.MotanResponse{RequestID: request.GetRequestID()}
	response.Value = rawResponseBytes
	return response
}

func (h *RPCWithHTTPProvider) extractUrl(r *http.Request) string {
	schema := "http://"
	if r.TLS != nil {
		schema = "https://"
	}
	return strings.Join([]string{schema, "127.0.0.1:" + strconv.Itoa(h.port), r.RequestURI}, "")
	//return strings.Join([]string{schema, r.Host, r.RequestURI}, "")
}
