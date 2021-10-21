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

type LinkChecker struct {
	Base       string
	HTTPClient *http.Client
}

func NewLinkChecker() (LinkChecker, error) {

	l := LinkChecker{
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}

	return l, nil
}

func (l LinkChecker) Get(url string) (Result, error) {

	resp, err := l.HTTPClient.Get(url)
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
