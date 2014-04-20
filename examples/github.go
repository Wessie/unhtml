// A small example on how to use unhtml to parse the GitHub commits page
//
// This is for example purpose only, use the GitHub API for actual programmatic
// access to GitHub.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Wessie/unhtml"
)

type GithubCommits struct {
	Commits []*Commit `unhtml:"descendant::*[@class='commit-group']/li"`
}

type Commit struct {
	Title  string    `unhtml:"p/a/@title"`
	Author string    `unhtml:"div//span[@rel='author']"`
	Time   time.Time `unhtml:"div//local-time/@datetime"`
	Sha1   string    `unhtml:"div//span[@class='sha']"`
}

func main() {
	// Find ourself some html
	resp, err := http.Get("https://github.com/Wessie/unhtml/commits/master")

	if err != nil {
		log.Fatal(err)
	}

	// Parse the HTML, accepts an io.Reader
	d, err := unhtml.NewDecoder(resp.Body)

	if err != nil {
		log.Fatal(err)
	}

	// Unmarshal into a struct with xpath tags for extracting things
	result := GithubCommits{}
	if err := d.Unmarshal(&result); err != nil {
		log.Fatal(err)
	}

	// Convert to JSON for pretty printing the result
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(b))
}
