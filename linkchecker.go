package linkchecker

import (
	"fmt"
	"net/http"
	"net/url"
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

func NewLinkChecker() (*LinkChecker, error) {

	l := &LinkChecker{
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

func (l LinkChecker) Check(sites []string) ([]Result, error) {

	results := []Result{}
	// parase the site
	for _, site := range sites {

		url, err := url.Parse(site)
		if err != nil {
			return []Result{}, err
		}
		if url.Scheme == "" || url.Host == "" {
			return []Result{}, fmt.Errorf("invalid URL %q", url)
		}

		result, err := l.Get(site)
		if err != nil {
			return []Result{}, fmt.Errorf("unable to perform Get on %s, %s", site, err)
		}
		results = append(results, result)

	}

	return results, nil
}
