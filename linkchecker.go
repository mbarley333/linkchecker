package linkchecker

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/antchfx/htmlquery"
	"golang.org/x/time/rate"
)

type Result struct {
	ResponseCode  int
	Url           string
	Error         error
	ReferringSite string
}

type LinkChecker struct {
	HTTPClient *http.Client
	Wg         sync.WaitGroup
	//Results    []Result
	Results     chan Result
	output      io.Writer
	errorLog    io.Writer
	Scheme      string
	Domain      string
	Ratelimiter *rate.Limiter
}

type Option func(*LinkChecker) error

func WithOutput(output io.Writer) Option {
	return func(l *LinkChecker) error {
		l.output = output
		return nil
	}
}

func WithErrorLog(errorLog io.Writer) Option {
	return func(l *LinkChecker) error {
		l.errorLog = errorLog
		return nil
	}
}

func NewLinkChecker(opts ...Option) (*LinkChecker, error) {

	linkchecker := &LinkChecker{
		HTTPClient:  &http.Client{Timeout: 10 * time.Second},
		output:      os.Stdout,
		errorLog:    os.Stderr,
		Ratelimiter: rate.NewLimiter(2, 4),
		Results:     make(chan Result, 10),
	}

	for _, o := range opts {
		o(linkchecker)
	}

	return linkchecker, nil
}

func (l *LinkChecker) Check(site string) error {

	url, err := url.Parse(site)
	if err != nil {
		return err
	}

	l.Scheme, l.Domain = url.Scheme, url.Host

	canonicalSite, err := l.CanonicaliseUrl(site)
	if err != nil {
		return err
	}

	already := NewAlreadyCrawled()
	already.AddSite(canonicalSite)

	l.Wg.Add(1)
	l.Crawl(canonicalSite, canonicalSite, l.Results, already)
	l.Wg.Wait()

	close(l.Results)

	return nil

}

func (l *LinkChecker) Crawl(site string, referringSite string, results chan<- Result, already *AlreadyCrawled) {

	result := Result{
		Url:           site,
		ReferringSite: referringSite,
	}

	resp, err := l.GetResponse(site)
	if err != nil {

		result.Error = err
		results <- result
		return

	}
	result.ResponseCode = resp.StatusCode

	u, err := url.Parse(site)
	if err != nil {
		result.Error = err
		results <- result
		return

	}

	//fmt.Println(result)
	results <- result
	l.Wg.Done()

	if u.Host == l.Domain {

		sites, err := l.ParseBody(resp.Body)

		if err != nil {
			fmt.Fprintf(l.errorLog, "unable to generate site list, %s", err)
		}

		for _, subsite := range sites {

			if !already.IsCrawled(subsite) {
				already.AddSite(subsite)
				l.Wg.Add(1)
				go l.Crawl(subsite, site, results, already)
				//l.Wg.Wait()

			}
		}

	}

}

func (l LinkChecker) GetResponse(site string) (*http.Response, error) {

	ctx := context.Background()
	err := l.Ratelimiter.Wait(ctx)
	if err != nil {
		return nil, err
	}

	_, err = l.IsHeaderAvailable(site)
	if err != nil {
		return nil, err
	}

	resp, err := l.HTTPClient.Get(site)
	if err != nil {

		return resp, err
	}

	return resp, nil
}

func (l LinkChecker) ParseBody(body io.Reader) ([]string, error) {

	sites := []string{}
	doc, err := htmlquery.Parse(body)
	if err != nil {
		return nil, fmt.Errorf("unable to parse body, check if a valid io.Reader is being sent, %s", err)
	}

	list := htmlquery.Find(doc, "//a/@href")

	for _, n := range list {
		href := htmlquery.InnerText(n)

		site, err := l.CanonicaliseUrl(href)
		if err != nil {
			fmt.Fprintf(l.errorLog, "unable to canonicalise url: %s, %s", site, err)
		}
		sites = append(sites, site)

	}

	return sites, nil
}

func (l LinkChecker) IsHeaderAvailable(site string) (bool, error) {

	resp, err := l.HTTPClient.Head(site)
	if err != nil {
		return false, err
	}

	defer resp.Body.Close()

	return true, nil
}

type AlreadyCrawled struct {
	Locker sync.Mutex
	List   map[string]bool
}

func (a *AlreadyCrawled) IsCrawled(site string) bool {

	result := false
	if a.List[site] {
		result = true
	}

	return result

}

func (a *AlreadyCrawled) AddSite(site string) {

	a.Locker.Lock()
	defer a.Locker.Unlock()
	a.List[site] = true

}

func NewAlreadyCrawled() *AlreadyCrawled {
	a := &AlreadyCrawled{
		List: map[string]bool{},
	}

	return a
}

func (l *LinkChecker) CanonicaliseUrl(site string) (string, error) {

	isUrl := false
	newUrl := site
	var err error

	// if no initial scheme from Check, try https and http
	if l.Scheme == "" {

		schemes := []string{"https", "http"}

		for _, scheme := range schemes {
			str := []string{scheme, "://", site}
			newUrl = strings.Join(str, "")
			isUrl, err = l.IsHeaderAvailable(newUrl)
			if err != nil {
				fmt.Fprintf(l.errorLog, "unable to use https scheme for %s, %s", site, err)
			}

			if isUrl {
				l.Scheme = scheme
				l.Domain = site
				break
			}

		}
		// crawled links
	} else {

		url, err := url.Parse(site)

		if err != nil {
			return "", err
		}

		scheme := ""
		if url.Scheme == "" {
			scheme = l.Scheme + "://"
		}

		//slash needed for intrasite links with w/o domain (e.g <a href="home">)
		slash := ""

		domain := ""
		if url.Host == "" {
			domain = l.Domain
			slash = "/"
		}

		// handle ./ link back to main page
		if site == "./" {
			site = ""
			slash = ""
		} else if strings.Index(site, "/") == 0 {
			site = RemoveLeadingSlash(site)
		}

		// assemble url
		str := []string{scheme, domain, slash, site}
		newUrl = strings.Join(str, "")

	}

	return newUrl, nil
}

func RemoveLeadingSlash(site string) string {

	for strings.Index(site, "/") == 0 {
		site = strings.TrimPrefix(site, "/")
	}

	return site
}

// CLI
func RunCLI() {

	arg := os.Args[1:2]
	if arg[0] == "help" {
		help()
		os.Exit(0)
	}

	site := arg[0]

	l, err := NewLinkChecker()
	if err != nil {
		fmt.Fprintln(l.errorLog, err)
	}
	done := make(chan bool)

	go func() {
		for {
			r, more := <-l.Results
			if more {
				fmt.Println("received result", r)
			} else {
				fmt.Println("received all jobs")
				done <- true
				return
			}
		}
	}()

	err = l.Check(site)
	if err != nil {
		fmt.Fprintln(l.errorLog, err)
	}
	<-done

}

func help() {
	fmt.Fprintln(os.Stderr, `
	Description:
	  linkchecker will crawl a site and return the status of each link on the site
	
	Parameters:
	  None
	
	Usage:
	./linkchecker https://example.com
	`)
}
