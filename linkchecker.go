package linkchecker

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
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
	output     io.Writer
	Scheme     string
	Domain     string
}

type Option func(*LinkChecker) error

func WithOutput(output io.Writer) Option {
	return func(l *LinkChecker) error {
		l.output = output
		return nil
	}
}

func NewLinkChecker(opts ...Option) (*LinkChecker, error) {

	linkchecker := &LinkChecker{
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
		output:     os.Stdout,
	}

	for _, o := range opts {
		o(linkchecker)
	}

	return linkchecker, nil
}

func (l *LinkChecker) Check(sites []string) ([]Result, error) {

	results := make(chan Result)
	go l.ReceiveResultChannel(results)

	for _, site := range sites {

		// get details from url
		url, err := url.Parse(site)
		if err != nil {
			return nil, err
		}
		if url.Scheme == "" || url.Host == "" {
			return nil, fmt.Errorf("invalid URL %q", url)
		}

		// needed for href w/o https://... used in ParseBody
		l.Scheme, l.Domain = url.Scheme, url.Host

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

	sites, err := l.ParseBody(resp.Body)
	if err != nil {
		return nil, err
	}

	return sites, nil
}

func (l LinkChecker) Get(url string, results chan<- Result) {

	// get is too heavy...need something to just
	// get headers to avoid the timeout error from espn.com

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

func (l LinkChecker) ParseBody(body io.Reader) ([]Site, error) {

	sites := []Site{}
	doc, err := htmlquery.Parse(body)
	if err != nil {
		return nil, fmt.Errorf("unable to parse body, check if a valid io.Reader is being sent, %s", err)
	}

	list := htmlquery.Find(doc, "//a/@href")

	for _, n := range list {
		href := htmlquery.InnerText(n)

		switch {
		case strings.HasPrefix(href, "/"):
			url := fmt.Sprintf("%s://%s%s", l.Scheme, l.Domain, href)
			site := Site{
				URL: url,
			}
			sites = append(sites, site)
		case strings.HasPrefix(href, "https://"):
			site := Site{
				URL: href,
			}
			sites = append(sites, site)
		case strings.HasPrefix(href, "http://"):
			site := Site{
				URL: href,
			}
			sites = append(sites, site)
		}

	}

	return sites, nil
}

// CLI

func RunCLI() error {

	flagSet := flag.NewFlagSet("flags", flag.ExitOnError)
	flagSet.Usage = help

	flagSet.Parse(os.Args[1:])
	if flagSet.NArg() < 1 {
		fmt.Println("Please list site(s) to link check (e.g. ./linkchecker https://bitfieldconsulting.com)")
		os.Exit(1)
	}
	sites := flagSet.Args()

	flag.Parse()

	l, err := NewLinkChecker()
	if err != nil {
		return err
	}

	results, err := l.Check(sites)
	if err != nil {
		return err
	}

	for _, result := range results {
		fmt.Fprintln(l.output, result)
	}

	return nil

}

func help() {
	fmt.Fprintln(os.Stderr, `
	Description:
	  linkchecker will crawl a site and return the status of each link on the site
	
	Parameters:
	  None
	
	Usage:
	./linkchecker https://google.com
	`)
}
