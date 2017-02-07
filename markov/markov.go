package markov

import (
	"math/rand"
	"strings"
	"unicode"
)

const (
	start = "\x02"
	end   = "\x03"
)

func GenFirstOrder(corpus []string) string {
	graph := make(map[string][]string)
	graph[start] = make([]string, 0)

	for _, line := range corpus {
		words := strings.Fields(line)
		for i, word := range words {
			words[i] = strings.Map(func(r rune) rune {
				if unicode.IsPunct(r) || r == '\x02' || r == '\x03' {
					return -1
				}
				return r
			}, strings.ToLower(word))
		}

		for i, word := range words {
			if i == 0 {
				graph[start] = append(graph[start], word)
			}
			if _, found := graph[word]; !found {
				graph[word] = make([]string, 0)
			}

			if i == len(words)-1 {
				graph[word] = append(graph[word], end)
			} else {
				graph[word] = append(graph[word], words[i+1])
			}
		}
	}

	var words []string
	word := start
	for {
		max := len(graph[word])
		if max < 1 {
			break
		}
		word = graph[word][rand.Intn(max)]
		if word == end {
			break
		}
		words = append(words, word)
	}
	return strings.Join(words, " ")
}
