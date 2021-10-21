package linkchecker

import (
	"fmt"
	"net/http"
	"time"
)

type Result struct {
	ResponseCode int
	Url          string
}

type Client struct {
	Base       string
	HTTPClient *http.Client
}

func NewClient() (Client, error) {

	c := Client{
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}

	return c, nil
}

func (c Client) Get(url string) (Result, error) {

	resp, err := c.HTTPClient.Get(url)
	if err != nil {
		return Result{}, fmt.Errorf("unable to perform get on %s,%s", url, err)
	}

	defer resp.Body.Close()

	result := Result{
		Url:          url,
		ResponseCode: resp.StatusCode,
	}

	return result, nil

}
