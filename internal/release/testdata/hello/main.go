// Command hello is a tiny program cross-compiled by the release tests.
package main

import "fmt"

var version = "dev"

func main() { fmt.Println("hello", version) }
