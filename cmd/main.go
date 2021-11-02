package main

import (
	"fmt"
	"linkchecker"
	"net/http"
)

func main() {
	//linkchecker.RunCLI()
	results := linkchecker.CheckSiteLinks("example.com",
		linkchecker.WithBufferedChannelSize(100),
	)

	for result := range results {

		// output only broken links
		if result.ResponseCode != http.StatusOK || result.Error != nil {
			fmt.Println(result)
		}
	}
}
