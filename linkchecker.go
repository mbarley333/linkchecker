package linkchecker

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/antchfx/htmlquery"
)

type Result struct {
	ResponseCode int
	Url          string
}

type LinkChecker struct {
	HTTPClient *http.Client
	Wg         sync.WaitGroup
	Results    []Result
}

func NewLinkChecker() (*LinkChecker, error) {

	l := &LinkChecker{
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}

	return l, nil
}

func (l *LinkChecker) Check(sites []string) ([]Result, error) {

	results := make(chan Result)
	go l.ReceiveResultChannel(results)

	// parse the site
	for _, site := range sites {

		subsites, err := l.Crawl(site)
		if err != nil {
			return nil, err
		}

		for _, subsite := range subsites {
			// add to wait group
			l.Wg.Add(1)
			go l.Get(subsite.URL, results)

		}

	}

	// block here until all wait groups handled
	l.Wg.Wait()
	return l.Results, nil
}

func (l LinkChecker) Crawl(url string) ([]Site, error) {
	resp, err := l.HTTPClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("unable to perform get on %s,%s", url, err)
	}

	defer resp.Body.Close()

	sites, err := ParseBody(resp.Body)
	if err != nil {
		return nil, err
	}

	return sites, nil
}

func (l LinkChecker) Get(url string, results chan<- Result) {

	resp, err := l.HTTPClient.Get(url)
	if err != nil {
		fmt.Println(err)
	}

	defer resp.Body.Close()

	result := Result{
		Url:          url,
		ResponseCode: resp.StatusCode,
	}

	results <- result

}

func (l *LinkChecker) ReceiveResultChannel(results <-chan Result) {

	for result := range results {
		l.Results = append(l.Results, result)
		l.Wg.Done()

	}

}

type Site struct {
	URL string
}

func ParseBody(body io.Reader) ([]Site, error) {

	sites := []Site{}
	doc, err := htmlquery.Parse(body)
	if err != nil {
		return nil, fmt.Errorf("unable to parse body, check if a valid io.Reader is being sent, %s", err)
	}

	list := htmlquery.Find(doc, "//a/@href")

	for _, n := range list {
		site := Site{
			URL: htmlquery.SelectAttr(n, "href"),
		}
		sites = append(sites, site)

	}

	return sites, nil
}
