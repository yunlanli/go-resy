package http

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/lgrees/resy-cli/constants"
	"github.com/spf13/viper"
)

func getAuthHeaders() *http.Header {
	apiKey := viper.GetString("resy_api_key")
	authToken := viper.GetString("resy_auth_token")
	return &http.Header{
		"authorization":         {fmt.Sprintf(`ResyAPI api_key="%s"`, apiKey)},
		"x-resy-auth-token":     {authToken},
		"x-resy-universal-auth": {authToken},
	}
}

type Req struct {
	QueryParams map[string]string
	Body        []byte
}

func template(method string, contentType string) func(string, *Req) ([]byte, int, error) {
	return func(url string, p *Req) ([]byte, int, error) {
		req, _ := http.NewRequest(method, url, bytes.NewReader(p.Body))
		client := &http.Client{Timeout: constants.HttpReqTimeout * time.Second}
		authHeaders := getAuthHeaders()
		if contentType != "" {
			req.Header.Add("content-type", contentType)
		}
		for key, val := range *authHeaders {
			req.Header.Add(key, val[0])
		}
		if p.QueryParams != nil {
			query := req.URL.Query()
			for key, val := range p.QueryParams {
				query.Add(key, val)
			}
			req.URL.RawQuery = query.Encode()
		}

		res, err := client.Do(req)

		if err != nil {
			return nil, 500, err
		}
		if res == nil {
			return nil, 0, nil
		}
		defer res.Body.Close()

		body, err := io.ReadAll(res.Body)

		if err != nil {
			return nil, res.StatusCode, err
		}

		return body, res.StatusCode, nil
	}
}

func PostJSON(url string, p *Req) ([]byte, int, error) {
	return template(http.MethodPost, "application/json")(url, p)
}

func PostForm(url string, p *Req) ([]byte, int, error) {
	return template(http.MethodPost, "application/x-www-form-urlencoded")(url, p)
}

func Get(url string, p *Req) ([]byte, int, error) {
	return template(http.MethodGet, "")(url, p)
}
