package cli

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
)

// linePick is the default picker: a numbered list on stderr, one line of
// answer on stdin (a number, or a key typed in full; empty cancels).
func (a *app) linePick(choices []Choice) (string, error) {
	if len(choices) > 9 {
		choices = choices[:9]
	}
	for i, c := range choices {
		fmt.Fprintf(a.env.Stderr, "  %d) %s  %s\n", i+1, c.Key, c.Summary)
	}
	fmt.Fprint(a.env.Stderr, "Start which? (number or KEY, empty cancels) ")
	line, _ := bufio.NewReader(a.env.Stdin).ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return "", nil
	}
	if n, err := strconv.Atoi(line); err == nil {
		if n < 1 || n > len(choices) {
			return "", fmt.Errorf("%d is not one of the choices", n)
		}
		return choices[n-1].Key, nil
	}
	return line, nil // core refuses a bad key with usage
}
