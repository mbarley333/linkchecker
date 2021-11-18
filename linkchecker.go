package linkchecker

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/antchfx/htmlquery"
	"golang.org/x/time/rate"
)

type Result struct {
	ResponseCode  int
	Url           string
	Problem       string
	ReferringSite string
	Status        Status
}

type LinkChecker struct {
	HTTPClient  *http.Client
	Wg          sync.WaitGroup
	Results     chan Result
	output      io.Writer
	errorLog    io.Writer
	Scheme      string
	Domain      string
	Ratelimiter *rate.Limiter
	CheckLink   CheckLink
	VerboseMode bool
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

func WithConfigureRatelimiter(ratePerSec rate.Limit, burst int) Option {
	return func(l *LinkChecker) error {
		l.Ratelimiter = rate.NewLimiter(ratePerSec, burst)
		return nil
	}
}

func WithBufferedChannelSize(size int) Option {
	return func(l *LinkChecker) error {
		l.Results = make(chan Result, size)
		return nil
	}
}

func WithVerboseMode() Option {
	return func(l *LinkChecker) error {
		l.VerboseMode = true
		return nil
	}
}

func NewLinkChecker(opts ...Option) (*LinkChecker, error) {

	linkchecker := &LinkChecker{
		HTTPClient:  &http.Client{Timeout: 10 * time.Second},
		output:      os.Stdout,
		errorLog:    os.Stderr,
		Ratelimiter: rate.NewLimiter(4, 4),
		Results:     make(chan Result, 1000),
		CheckLink: CheckLink{
			List: make(map[string]bool),
		},
	}

	for _, o := range opts {
		o(linkchecker)
	}

	return linkchecker, nil
}

func CheckSiteLinks(site string, opts ...Option) <-chan Result {
	l, err := NewLinkChecker()
	for _, o := range opts {
		o(l)
	}

	if err != nil {
		fmt.Fprintf(l.errorLog, "unable to create linkchecker struct, %s", err)
	}

	err = l.Check(site)
	if err != nil {
		fmt.Fprint(l.errorLog, err)
	}

	return l.Results

}

func (l *LinkChecker) GetAllResults() []Result {

	results := []Result{}

	if l.VerboseMode {
		for result := range l.Results {
			results = append(results, result)
		}
	} else if !l.VerboseMode {
		for result := range l.Results {
			if result.Status != StatusUp {
				results = append(results, result)
			}
		}
	}

	sort.Slice(results, func(i, j int) bool { return results[i].Url < results[j].Url })

	return results

}

func (l *LinkChecker) Check(site string) error {

	url, err := url.Parse(strings.TrimSpace(site))
	if err != nil {
		return err
	}

	l.Scheme, l.Domain = url.Scheme, url.Host

	canonicalSite, err := l.CanonicaliseUrl(site)
	if err != nil {
		return err
	}

	referringSite := canonicalSite

	l.Wg.Add(1)
	go l.Crawl(canonicalSite, referringSite)
	l.Wg.Wait()

	close(l.Results)

	return nil

}

func (l *LinkChecker) Crawl(site string, referringSite string) {

	defer l.Wg.Done()

	if l.IsCrawled(site) {
		return
	}

	result := Result{
		Url:           site,
		ReferringSite: referringSite,
	}

	l.AddSite(site)

	// check headers
	code, err := l.HeadStatus(site)
	if err != nil {
		result.Problem = err.Error()
		result.Status = StatusDown
		l.Results <- result
		return
	}

	// check for http.StatusOK
	if code == http.StatusTooManyRequests {
		result.Problem = "Site rate limit exceeded"
		result.ResponseCode = code
		result.Status = StatusRateLimited
		l.Results <- result
		return
	}

	// handle 999 error code
	if code == 999 {
		result.Problem = "Non standard error returned by external service"
		result.ResponseCode = code
		result.Status = Status999
		l.Results <- result
		return
	}

	// check if able to parse site
	u, err := url.Parse(site)
	if err != nil {
		result.Problem = err.Error()
		l.Results <- result
		return
	}

	// external site
	if u.Host != l.Domain {

		result.Status = StatusUp
		result.ResponseCode = code
		l.Results <- result
		return
	}

	request, err := http.NewRequest("GET", site, nil)
	if err != nil {
		fmt.Fprintln(l.output, err)
	}
	request.Header.Set("user-agent", "linkchecker")
	request.Header.Set("accept", "*/*")

	resp, err := l.HTTPClient.Do(request)
	if err != nil {
		result.Problem = err.Error()
		result.Status = StatusDown
		l.Results <- result
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		result.Problem = "Non OK response"
		result.ResponseCode = resp.StatusCode
		result.Status = StatusDown
		l.Results <- result
		return
	}

	result.Status = StatusUp
	result.ResponseCode = resp.StatusCode

	l.Results <- result

	// generate of list of links on page
	links, err := l.ParseBody(resp.Body)

	if err != nil {
		fmt.Fprintf(l.errorLog, "unable to generate site list, %s", err)
	}

	for _, link := range links {

		if !l.IsCrawled(link) {

			ctx := context.Background()
			err := l.Ratelimiter.Wait(ctx)

			l.Wg.Add(1)

			if err != nil {
				fmt.Fprintln(l.errorLog, err)
			}
			go l.Crawl(link, site)

		}

	}

}

func (l *LinkChecker) GetResponse(site string) (*http.Response, error) {

	_, err := l.HeadStatus(site)
	if err != nil {
		return nil, err
	}

	resp, err := l.HTTPClient.Get(site)
	if err != nil {

		return resp, err
	}

	return resp, nil
}

func (l *LinkChecker) ParseBody(body io.Reader) ([]string, error) {

	sites := []string{}
	doc, err := htmlquery.Parse(body)
	if err != nil {
		return nil, fmt.Errorf("unable to parse body, check if a valid io.Reader is being sent, %s", err)
	}

	list := htmlquery.Find(doc, "//a/@href")

	for _, n := range list {
		url := htmlquery.InnerText(n)

		if l.IsLinkOkToAdd(url) {
			url, err = l.CanonicaliseChildUrl(url)
			if err != nil {
				fmt.Fprintf(l.errorLog, "unable to canonicalise url: %s, %s", url, err)
			}
			sites = append(sites, url)
		}

	}

	return sites, nil
}

func (l *LinkChecker) IsLinkOkToAdd(link string) bool {

	u, err := url.Parse(link)
	if err != nil {
		fmt.Fprintln(l.output, err)
	}

	// filter out mailto:, ftp: and localhost type links
	return !strings.HasPrefix(strings.ToLower(link), "mailto:") && !strings.HasPrefix(strings.ToLower(link), "ftp:") && !(u.Hostname() == "localhost" && !strings.HasPrefix(strings.ToLower(l.Domain), "localhost"))
}

func (l *LinkChecker) HeadStatus(site string) (int, error) {

	request, err := http.NewRequest("HEAD", site, nil)
	if err != nil {
		fmt.Println(err)
	}
	request.Header.Set("user-agent", "linkchecker")
	request.Header.Set("accept", "*/*")

	resp, err := l.HTTPClient.Do(request)

	if err != nil {
		return 0, err
	}

	defer resp.Body.Close()

	return resp.StatusCode, nil
}

type CheckLink struct {
	mutex sync.RWMutex
	List  map[string]bool
}

func (l *LinkChecker) IsCrawled(site string) bool {

	l.CheckLink.mutex.RLock()
	defer l.CheckLink.mutex.RUnlock()

	result := l.CheckLink.List[site]

	return result
}

func (l *LinkChecker) AddSite(site string) {

	l.CheckLink.mutex.Lock()
	defer l.CheckLink.mutex.Unlock()
	l.CheckLink.List[site] = true

}

func (l *LinkChecker) CanonicaliseUrl(site string) (string, error) {

	newUrl := strings.TrimSpace(site)

	// if no initial scheme from Check, try https and http
	if l.Scheme == "" {

		schemes := []string{"https", "http"}

		for _, scheme := range schemes {
			str := []string{scheme, "://", site}
			newUrl = strings.Join(str, "")
			code, err := l.HeadStatus(newUrl)
			if err != nil {
				fmt.Fprintf(l.errorLog, "unable to use https scheme for %s, %s", site, err)
			}

			if code == http.StatusOK {
				u, err := url.Parse(newUrl)
				if err != nil {
					fmt.Fprintf(l.errorLog, "unable to parse url %s, %s", newUrl, err)
				}
				l.Scheme = u.Scheme
				l.Domain = u.Host
				break
			}

		}

	}

	return newUrl, nil
}

func (l *LinkChecker) CanonicaliseChildUrl(site string) (string, error) {

	canonical := strings.TrimSpace(site)
	var err error

	if canonical == "./" {
		// set to empty string and just use domain name
		canonical = l.Scheme + "://" + l.Domain

	} else if strings.HasPrefix(site, "/") {
		canonical = RemoveLeadingSlash(canonical)
	}

	url, err := url.Parse(canonical)
	if err != nil {
		return "", err
	}

	if url.Host == "" {
		canonical = l.Domain + "/" + canonical
	}

	if url.Scheme == "" {
		canonical = l.Scheme + "://" + canonical
	}

	return canonical, nil
}

func RemoveLeadingSlash(site string) string {

	for strings.Index(site, "/") == 0 {
		site = strings.TrimPrefix(site, "/")
	}

	return site
}

type Status int

var StatusStringMap = map[Status]string{
	None:              "Invalid Status",
	StatusUp:          "Up",
	StatusDown:        "Down",
	StatusRateLimited: "RateLimited",
	Status999:         "Unable to verify",
}

func (s Status) String() string {
	return StatusStringMap[s]
}

const (
	None Status = iota
	StatusUp
	StatusDown
	StatusRateLimited
	Status999
)

var HttpStatusMap = map[int]Status{
	http.StatusAccepted:        StatusUp,
	http.StatusOK:              StatusUp,
	http.StatusCreated:         StatusUp,
	http.StatusTooManyRequests: StatusRateLimited,
	999:                        Status999,
}

type Color string

const (
	ColorRed    Color = "\u001b[31;1m"
	ColorGreen  Color = "\033[32m"
	ColorYellow Color = "\u001b[33;1m"
	ColorReset  Color = "\033[0m"
)

var StatusColorMap = map[Status]Color{
	StatusUp:          ColorGreen,
	StatusDown:        ColorRed,
	StatusRateLimited: ColorYellow,
	Status999:         ColorYellow,
}

func (r Result) String() string {

	status := None

	resp, ok := HttpStatusMap[r.ResponseCode]

	if !ok {
		status = StatusDown
	} else {
		status = resp
	}

	color := StatusColorMap[status]

	str := []string{string(color), "URL: ", r.Url, " \nStatus: ", r.Status.String(), "\nStatus Code: ", strconv.Itoa(r.ResponseCode), " \nProblem: ", r.Problem, "\nReferring URL: ", r.ReferringSite, "\n", string(ColorReset)}

	return strings.Join(str, "")
}

// CLI
func RunCLI() {

	if len(os.Args) < 2 {
		help(os.Args[0])
		os.Exit(1)
	}

	site := os.Args[1]
	if site == "help" {
		fmt.Println(os.Args[0])
		help(os.Args[0])
		os.Exit(0)
	}

	l, err := NewLinkChecker()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}

	defer l.elapsed("linkchecker")()

	err = l.Check(site)
	if err != nil {
		fmt.Fprintln(l.errorLog, err)
	}

	results := l.GetAllResults()

	for _, result := range results {
		fmt.Fprintln(l.output, result)
	}

}

func help(cliArg string) {

	arg := "./linkchecker"

	// docker run
	if cliArg == "/bin/linkchecker" {
		arg = "docker run mbarley333/linkchecker:[tag]"
	}

	fmt.Fprintf(os.Stderr, `
	Description:
	  linkchecker will crawl a site and return the status of each link on the site
	
	Usage:
	%s https://somewebpage123.com
	`, arg)
}

func (l *LinkChecker) elapsed(what string) func() {
	start := time.Now()
	return func() {
		fmt.Fprintf(l.output, "%s completed in %v\n", what, time.Since(start))
	}
}
