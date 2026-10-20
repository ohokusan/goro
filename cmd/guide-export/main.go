// guide-export writes only public game metadata; it never exports account settings.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/kivutar/goro/guide"
	"os"
)

func main() {
	root := flag.String("server-root", "~/rathena", "local rAthena source directory")
	mode := flag.String("mode", "auto", "auto, pre-re or re")
	flag.Parse()
	s, err := guide.Load(*root, *mode)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	e := json.NewEncoder(os.Stdout)
	e.SetIndent("", "  ")
	if err = e.Encode(s); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
