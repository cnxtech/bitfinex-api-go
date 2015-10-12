package bitfinex

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"
)

//TODO: use var instead of const
const (
	BaseURL      = "https://api.bitfinex.com/v1/"
	WebSocketURL = "wss://api2.bitfinex.com:3000/ws"
)

type Client struct {
	// HTTP client used to communicate with the API.
	httpClient *http.Client
	// Base URL for API requests.
	BaseURL *url.URL

	// Auth data
	ApiKey    string
	ApiSecret string

	// Services
	Pairs      *PairsService
	Account    *AccountService
	Balances   *BalancesService
	Credits    *CreditsService
	Lendbook   *LendbookService
	MarginInfo *MarginInfoServive
	OrderBook  *OrderBookServive
	Orders     *OrderService
	WebSocket  *WebSocketService
}

// NewClient creates new Bitfinex.com API http client
func NewClient() *Client {
	baseURL, _ := url.Parse(BaseURL)

	c := &Client{httpClient: http.DefaultClient, BaseURL: baseURL}
	c.Pairs = &PairsService{client: c}
	c.Account = &AccountService{client: c}
	c.Balances = &BalancesService{client: c}
	c.Credits = &CreditsService{client: c}
	c.Lendbook = &LendbookService{client: c}
	c.MarginInfo = &MarginInfoServive{client: c}
	c.OrderBook = &OrderBookServive{client: c}
	c.Orders = &OrderService{client: c}
	c.WebSocket = NewWebSocketService(c)

	return c
}

// NewRequest create new API request. Relative url can be provided in refUrl.
func (c *Client) NewRequest(method string, refUrl string, values url.Values) (*http.Request, error) {
	rel, err := url.Parse(refUrl)
	if err != nil {
		return nil, err
	}

	var req *http.Request
	u := c.BaseURL.ResolveReference(rel)
	if values != nil {
		req, err = http.NewRequest(method, u.String(), nil)
	} else {
		req, err = http.NewRequest(method, u.String(), strings.NewReader(values.Encode()))
	}

	if err != nil {
		return nil, err
	}

	return req, nil
}

// NewAuthenticatedRequest creates new http request for authenticated routes
func (c *Client) NewAuthenticatedRequest(m string, refUrl string, values url.Values) (*http.Request, error) {
	req, err := c.NewRequest(m, refUrl, values)
	if err != nil {
		return nil, err
	}

	payload := map[string]string{
		"request": "/v1/" + refUrl,
		"nonce":   fmt.Sprintf("%v", time.Now().Unix()*10000),
	}

	payload_json, _ := json.Marshal(payload)
	payload_enc := base64.StdEncoding.EncodeToString(payload_json)

	sig := hmac.New(sha512.New384, []byte(c.ApiSecret))
	sig.Write([]byte(payload_enc))

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	req.Header.Add("X-BFX-APIKEY", c.ApiKey)
	req.Header.Add("X-BFX-PAYLOAD", payload_enc)
	req.Header.Add("X-BFX-SIGNATURE", hex.EncodeToString(sig.Sum(nil)))

	return req, nil
}

// Auth sets api key and secret for usage is requests that
// requires authentication
func (c *Client) Auth(key string, secret string) *Client {
	c.ApiKey = key
	c.ApiSecret = secret

	return c
}

// Do executes API request created by NewRequest method or custom *http.Request.
func (c *Client) Do(req *http.Request, v interface{}) (*Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	response := newResponse(resp)
	// fmt.Println("API RESP:", response.String())

	err = checkResponse(resp)
	if err != nil {
		// Return response in case caller need to debug it.
		return response, err
	}

	if v != nil {
		err = json.NewDecoder(resp.Body).Decode(v)
		if err != nil {
			return nil, err
		}
	}

	return response, nil
}

// Response is wrapper for standard http.Response and provides
// more methods.
type Response struct {
	Response *http.Response
}

// newResponse creates new wrapper.
func newResponse(r *http.Response) *Response {
	return &Response{Response: r}
}

// String converts response body to string.
// An empty string will be returned if error.
func (r *Response) String() string {
	body, err := ioutil.ReadAll(r.Response.Body)
	if err != nil {
		return ""
	}

	return string(body)
}

// In case if API will wrong response code
// ErrorResponse will be returned to caller
type ErrorResponse struct {
	Response *http.Response
	Message  string `json:"message"`
}

func (r *ErrorResponse) Error() string {
	return fmt.Sprintf("%v %v: %d %v",
		r.Response.Request.Method,
		r.Response.Request.URL,
		r.Response.StatusCode,
		r.Message,
	)
}

// checkResponse checks response status code and response
// for errors.
func checkResponse(r *http.Response) error {
	if c := r.StatusCode; 200 <= c && c <= 299 {
		return nil
	}

	// Try to decode error message
	errorResponse := &ErrorResponse{Response: r}
	err := json.NewDecoder(r.Body).Decode(errorResponse)
	if err != nil {
		errorResponse.Message = "Error decoding response error message. " +
			"Please see response body for more information."
	}

	return errorResponse
}