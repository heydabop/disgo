package markov

import (
	mrand "math/rand"
	"strings"
	"time"
)

const (
	zero      = "\x00"
	start     = "\x02"
	end       = "\x03"
	zeroRune  = rune('\x00')
	startRune = rune('\x02')
	endRune   = rune('\x02')
)

var (
	rand        = mrand.New(mrand.NewSource(time.Now().UnixNano()))
	startPair   = [2]string{zero, start}
	startTriple = [3]string{zero, start, start}
)

func prune(words []string) []string {
	for i, word := range words {
		words[i] = strings.Map(func(r rune) rune {
			if r == startRune || r == endRune || r == zeroRune {
				return -1
			}
			return r
		}, word)
	}

	return words
}

//GenFirstOrder returns a random sentence generated from first-order markov chains made from the input corpus
func GenFirstOrder(corpus []string) string {
	graph := make(map[string][]string)
	graph[start] = make([]string, 0)

	for _, line := range corpus {
		words := prune(strings.Fields(line))

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

//GenSecondOrder returns a random sentence generated from first-order markov chains made from the input corpus
func GenSecondOrder(corpus []string) string {
	graph := make(map[[2]string][][2]string)
	graph[startPair] = make([][2]string, 0)

	for _, line := range corpus {
		words := prune(strings.Fields(line))

		for i, word := range words {
			prevWord := start
			nextWord := end
			if i > 0 {
				prevWord = words[i-1]
			}
			if i < len(words)-1 {
				nextWord = words[i+1]
			}
			prevPair := [2]string{prevWord, word}
			nextPair := [2]string{word, nextWord}

			if _, found := graph[prevPair]; !found {
				graph[prevPair] = make([][2]string, 0)
			}

			if i == 0 {
				graph[startPair] = append(graph[startPair], prevPair)
			}
			graph[prevPair] = append(graph[prevPair], nextPair)
		}
	}

	var words []string
	pair := startPair
	for {
		max := len(graph[pair])
		if max < 1 {
			break
		}
		pair = graph[pair][rand.Intn(max)]
		if pair[1] == end {
			break
		}
		words = append(words, pair[1])
	}
	return strings.Join(words, " ")
}

//GenThirdOrder returns a random sentence generated from first-order markov chains made from the input corpus
func GenThirdOrder(corpus []string) string {
	graph := make(map[[3]string][][3]string)
	graph[startTriple] = make([][3]string, 0)

	for _, line := range corpus {
		words := prune(strings.Fields(line))

		for i, word := range words {
			prevWords := []string{start, start}
			nextWord := end
			if i > 0 {
				prevWords[1] = words[i-1]
				if i > 1 {
					prevWords[0] = words[i-2]
				}
			}
			if i < len(words)-1 {
				nextWord = words[i+1]
			}
			prevTriple := [3]string{prevWords[0], prevWords[1], word}
			nextTriple := [3]string{prevWords[1], word, nextWord}

			if _, found := graph[prevTriple]; !found {
				graph[prevTriple] = make([][3]string, 0)
			}

			if i == 0 {
				graph[startTriple] = append(graph[startTriple], prevTriple)
			}
			graph[prevTriple] = append(graph[prevTriple], nextTriple)
		}
	}

	var words []string
	triple := startTriple
	for {
		max := len(graph[triple])
		if max < 1 {
			break
		}
		triple = graph[triple][rand.Intn(max)]
		if triple[2] == end {
			break
		}
		words = append(words, triple[2])
	}
	return strings.Join(words, " ")
}
