package linkchecker

import (
	"context"
	"flag"
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

type LinkChecker struct {
	// exported fields user wants
	Domain string
	Scheme string

	// exported fields needed for testing
	HTTPClient  *http.Client
	ProgressBar *Bar

	// unexported
	results     chan Result
	wg          sync.WaitGroup
	output      io.Writer
	errorLog    io.Writer
	checkLink   CheckLink
	verboseMode bool
	ratelimiter *rate.Limiter
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

func WithLinkcheckerSpeed(speed CheckSpeed) Option {
	return func(l *LinkChecker) error {
		result := GetCheckSpeed(speed)
		l.ratelimiter = rate.NewLimiter(rate.Limit(result.Rate), result.Burst)
		return nil
	}
}

func WithConfigureRatelimiter(ratePerSec rate.Limit, burst int) Option {
	return func(l *LinkChecker) error {
		l.ratelimiter = rate.NewLimiter(ratePerSec, burst)
		return nil
	}
}

func WithBufferedChannelSize(size int) Option {
	return func(l *LinkChecker) error {
		l.results = make(chan Result, size)
		return nil
	}
}

func WithVerboseMode() Option {
	return func(l *LinkChecker) error {
		l.verboseMode = true
		return nil
	}
}

func WithSilentMode() Option {
	return func(l *LinkChecker) error {
		l.errorLog = io.Discard
		return nil
	}
}

func WithProgressBar() Option {
	return func(l *LinkChecker) error {
		l.ProgressBar = NewBar(
			WithOutputBar(l.output),
		)
		return nil
	}
}

func NewLinkChecker(opts ...Option) (*LinkChecker, error) {

	linkchecker := &LinkChecker{
		HTTPClient:  &http.Client{Timeout: 10 * time.Second},
		output:      os.Stdout,
		errorLog:    os.Stderr,
		ratelimiter: rate.NewLimiter(2, 2),
		results:     make(chan Result, 2000),
		checkLink: CheckLink{
			list: make(map[string]bool),
		},
	}

	for _, o := range opts {
		o(linkchecker)
	}

	return linkchecker, nil
}

func (l *LinkChecker) StreamResults() <-chan Result {

	return l.results
}

func (l *LinkChecker) GetAllResults() []Result {

	results := []Result{}

	if l.verboseMode {
		for result := range l.results {
			results = append(results, result)
		}
	} else if !l.verboseMode {
		for result := range l.results {
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

	// check if progress bar enabled in LinkChecker struct
	if l.ProgressBar != nil {
		l.ProgressBar.Add()
	}

	l.wg.Add(1)
	go l.Crawl(canonicalSite, referringSite)
	l.wg.Wait()

	close(l.results)

	return nil

}

func (l *LinkChecker) Crawl(site string, referringSite string) {

	defer l.wg.Done()

	// check if progress bar enabled in LinkChecker struct
	if l.ProgressBar != nil {
		l.ProgressBar.Completed()
	}

	if l.IsCrawled(site) {
		return
	}

	result := Result{
		Url:           site,
		ReferringSite: referringSite,
	}

	l.AddSite(site)

	// check if able to parse site
	u, err := url.Parse(site)
	if err != nil {
		result.Problem = err.Error()
		l.results <- result
		return
	}

	// check head request first
	code, err := l.HeadStatus(site)
	if err != nil {

		if os.IsTimeout(err) {
			//if IsTimeout(err) {
			result.Problem = "Client.Timeout exceeded while awaiting headers"
			result.Status = StatusRateLimited
		} else {
			result.Problem = err.Error()
			result.Status = StatusDown
		}

		result.ResponseCode = code
		l.results <- result
		return
	}

	if code == http.StatusTooManyRequests {
		result.Problem = "Site rate limit exceeded"
		result.ResponseCode = code
		result.Status = StatusRateLimited
		l.results <- result
		return
	}

	// handle non stardard 999 error code for linkedin
	// and other services
	if code == 999 && strings.Contains(u.Host, "linkedin.com") {
		result.Problem = "linkedin is up, but rejects http requests"
		result.ResponseCode = code
		result.Status = StatusUp
		l.results <- result
		return
	} else if code == 999 {
		result.Problem = "Non standard error returned by external service"
		result.ResponseCode = code
		result.Status = Status999
		l.results <- result
		return
	}

	resp, err := l.GetResponse(site)
	if err != nil {
		//if IsTimeout(err) {
		if os.IsTimeout(err) {
			result.Problem = "Client.Timeout exceeded while awaiting headers"
			result.Status = StatusRateLimited
		} else {
			result.Problem = err.Error()
			result.Status = StatusDown
		}

		//result.ResponseCode = code
		l.results <- result
		return

	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		result.Problem = "Non OK response"
		result.ResponseCode = resp.StatusCode
		result.Status = StatusDown
		l.results <- result
		return
	}

	// external site
	if u.Host != l.Domain {
		result.Status = StatusUp
		result.ResponseCode = resp.StatusCode
		l.results <- result
		return
	}

	result.Status = StatusUp
	result.ResponseCode = resp.StatusCode

	l.results <- result

	// generate of list of links on page
	links, err := l.ParseBody(resp.Body)

	if err != nil {
		fmt.Fprintf(l.errorLog, "unable to generate site list, %s", err)
	}

	for _, link := range links {

		if !l.IsCrawled(link) {

			// check if progress bar enabled in LinkChecker struct
			if l.ProgressBar != nil {
				l.ProgressBar.Add()
			}

			ctx := context.Background()
			err := l.ratelimiter.Wait(ctx)

			l.wg.Add(1)

			if err != nil {
				fmt.Fprintln(l.errorLog, err)
			}
			go l.Crawl(link, site)
		}
	}
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
		fmt.Fprintln(l.errorLog, err)
	}

	// filter out mailto:, ftp: and localhost type links
	return !strings.HasPrefix(strings.ToLower(link), "mailto:") && !strings.HasPrefix(strings.ToLower(link), "ftp:") && !(u.Hostname() == "localhost" && !strings.HasPrefix(strings.ToLower(l.Domain), "localhost"))
}

func (l *LinkChecker) HeadStatus(link string) (int, error) {

	request, err := http.NewRequest(http.MethodHead, link, nil)
	if err != nil {
		fmt.Fprintln(l.errorLog, err)
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

func (l *LinkChecker) GetResponse(link string) (*http.Response, error) {

	request, err := http.NewRequest(http.MethodGet, link, nil)
	if err != nil {
		fmt.Fprintln(l.errorLog, err)
	}
	request.Header.Set("user-agent", "linkchecker")
	request.Header.Set("accept", "*/*")

	resp, err := l.HTTPClient.Do(request)

	if err != nil {
		return &http.Response{}, err
	}

	return resp, nil
}

type CheckLink struct {
	mutex sync.RWMutex
	list  map[string]bool
}

func (l *LinkChecker) IsCrawled(site string) bool {

	l.checkLink.mutex.RLock()
	defer l.checkLink.mutex.RUnlock()

	result := l.checkLink.list[site]

	return result
}

func (l *LinkChecker) AddSite(site string) {

	l.checkLink.mutex.Lock()
	defer l.checkLink.mutex.Unlock()
	l.checkLink.list[site] = true
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

func (l *LinkChecker) elapsed(what string) func() {
	start := time.Now()
	return func() {
		fmt.Fprintf(l.output, "\n%s completed in %v\n", what, time.Since(start))
	}
}

// Result is a struct that contains the status code and url of a link
type Result struct {
	ResponseCode  int
	Url           string
	Problem       string
	ReferringSite string
	Status        Status
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

	return l.results

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

	flagSet := flag.NewFlagSet("flags", flag.ExitOnError)
	slow := flagSet.Bool("slow", false, "linkchecker rate set to 1 request per second")
	normal := flagSet.Bool("normal", false, "linkchecker rate set two 2 requests per second")
	fast := flagSet.Bool("fast", false, "linkchecker rate set to 10 requests per second")
	furious := flagSet.Bool("furious", false, "linkchecker rate set to 20 requests per second")
	warp := flagSet.Bool("warp", false, "linkchecker rate set to 100 requests per second")

	if len(os.Args) < 2 {
		help(os.Args[0])
		os.Exit(1)
	}

	flagSet.Parse(os.Args[2:])

	var speed CheckSpeed

	if *normal || len(os.Args) < 3 {
		speed = CheckSpeedNormal
	} else if *slow {
		speed = CheckSpeedSlow
	} else if *fast {
		speed = CheckSpeedFast
	} else if *furious {
		speed = CheckSpeedFurious
	} else if *warp {
		speed = CheckSpeedWarp
	}

	_, ok := CheckSpeedMap[speed]

	site := os.Args[1]
	if site == "help" || !ok {
		fmt.Println(os.Args[0])
		help(os.Args[0])
		os.Exit(0)
	}

	l, err := NewLinkChecker(
		WithProgressBar(),
		WithLinkcheckerSpeed(speed),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}

	defer l.elapsed("linkchecker")()

	go l.ProgressBar.Refresher()

	go func() {
		for result := range l.StreamResults() {
			if !l.verboseMode {
				if result.Status != StatusUp {
					fmt.Fprintln(l.output, result)
				}
			} else {
				fmt.Fprintln(l.output, result)
			}

		}
	}()

	err = l.Check(site)
	if err != nil {
		fmt.Fprintln(l.errorLog, err)
	}

	close(l.ProgressBar.done)

}

func help(cliArg string) {

	arg := "./linkchecker"

	// docker run
	if cliArg == "/bin/linkchecker" {
		arg = "docker run mbarley333/linkchecker:[tag]"
	}

	fmt.Fprintf(os.Stderr, `
	Description:
	  Linkchecker will crawl a site and return the status of each link on the site. 
	  Use the optional flags to set linkchecker speed.  Please note that using linkchecker 
	  may trigger site ratelimiters if ratelimter thresholds are exceeded by linkchecker speed settings.
	
	Flags:
	  -normal: sets the linkchecker rate set to 2 request per second.  default speed if no flag is used.
	  -slow: sets the linkchecker rate set to 1 request per second.
	  -fast: sets the linkchecker rate set to 10 requests per second.
	  -furious: sets the linkchecker rate set to 20 requests per second.
	  -warp: sets the linkchecker rate set to 100 requests per second.

	Usage:
	%s https://somewebpage123.com
	`, arg)
}

type CheckSpeed int

const (
	CheckSpeedNone CheckSpeed = iota
	CheckSpeedSlow
	CheckSpeedNormal
	CheckSpeedFast
	CheckSpeedFurious
	CheckSpeedWarp
)

// var CheckSpeedFromStringMap = map[string]CheckSpeed{
// 	"slow":    CheckSpeedSlow,
// 	"normal":  CheckSpeedNormal,
// 	"fast":    CheckSpeedFast,
// 	"furious": CheckSpeedFurious,
// 	"warp":    CheckSpeedWarp,
// }

type LinkcheckSpeed struct {
	Rate  int
	Burst int
}

var CheckSpeedMap = map[CheckSpeed]LinkcheckSpeed{
	CheckSpeedSlow:    {Rate: 1, Burst: 1},
	CheckSpeedNormal:  {Rate: 2, Burst: 2},
	CheckSpeedFast:    {Rate: 10, Burst: 10},
	CheckSpeedFurious: {Rate: 20, Burst: 20},
	CheckSpeedWarp:    {Rate: 100, Burst: 100},
}

func GetCheckSpeed(speed CheckSpeed) LinkcheckSpeed {
	return CheckSpeedMap[speed]
}

func IsTimeout(err error) bool {

	var u *url.Error

	// assert type for error interface
	u, ok := err.(*url.Error)
	if !ok {
		return false
	}

	return u.Timeout()
}
