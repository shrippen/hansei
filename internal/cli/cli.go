// Package cli holds commands that optional build parts (the demo build) add to hansei.
package cli

// Command is an extra subcommand.
type Command struct {
	Usage string
	Run   func(args []string) error
}

// Extra is filled by packages compiled in with build tags.
var Extra = map[string]Command{}
