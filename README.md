# linkchecker

linkchecker is a Go library that will crawl a given web domain and test all urls within the domain.  linkchecker is available as a standalone binary as well.

Built with Aloha in Hawaii 🌊

Thank you to @bitfield for all of his mentoring on my Go journey!


# Installation as a library
1) From your project folder:
```bash
go get github.com/mbarley333/linkchecker
```
2) Usage:
```bash
import (
	"fmt"
	"net/http"

	"github.com/mbarley333/linkchecker"
)

func main() {
	results := linkchecker.CheckSiteLinks("https://somewebpage123.com")

	for result := range results {

		// output only broken links
		if result.ResponseCode != http.StatusOK || result.Error != nil {
			fmt.Println(result)
		}
	}

}
```




# Installation as a container (linux, mac)
```bash
docker pull mbarley333/linkchecker:latest
```
# Usage as a container
```bash
docker run mbarley333/linkchecker:latest https://somewebpage123.com
```




# Installation as a binary
* If you use a Mac, just curl the install.sh file
```bash
curl https://raw.githubusercontent.com/mbarley333/linkchecker/main/install.sh | sh
```
* All OS types just download the prebuilt binaries for your OS from the Releases section
  * Unzip
  * For Mac on first usage, open Finder and locate the unzipped file
	  Right Click on file > Open
  * cd to folder
  ```bash
  ./linkchecker https://somewebpage123.com
  ```

# Usage as a binary
```bash
./linkchecker -help

        Description:
          linkchecker will crawl a site and return the status of each link on the site

        Usage:
        ./linkchecker https://somewebpage123.com

```


# Functional Options!!!
* Ratelimiter
* Buffered Channel size for results
* Error log
* Output
