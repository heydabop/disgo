package hangman

import (
	"bytes"
	"fmt"
	"math/rand"
	"strings"
	"time"
)

const maxMinutesBetweenGuesses = 2

var maxWrongGuesses = len(boards) - 1

type Game struct {
	answer              string
	correctGuesses      []bool
	numWrongGuesses     int
	usedLetters         []byte
	authorID            string
	authorLastGuessTime time.Time
}

func NewGame(authorID string) *Game {
	answer := wordlist[rand.New(rand.NewSource(time.Now().UnixNano())).Intn(len(wordlist))]
	return &Game{
		answer:              answer,
		correctGuesses:      make([]bool, len(answer)),
		numWrongGuesses:     0,
		usedLetters:         make([]byte, 0),
		authorID:            authorID,
		authorLastGuessTime: time.Now(),
	}
}

func (g *Game) Guess(guesserID string, guess byte) (bool, error) {
	guess = bytes.ToLower([]byte{guess})[0]
	if guesserID != g.authorID && time.Since(g.authorLastGuessTime) < maxMinutesBetweenGuesses*time.Minute {
		return false, fmt.Errorf("You can't guess unless you started the game or it's been %d minutes since the last guess.", maxMinutesBetweenGuesses)
	}
	correctGuess := false
	for i := range g.answer {
		if g.answer[i] == guess {
			correctGuess = true
			g.correctGuesses[i] = true
		}
	}
	if guesserID == g.authorID {
		g.authorLastGuessTime = time.Now()
	}
	if correctGuess {
		return true, nil
	}
	g.numWrongGuesses++
	g.usedLetters = append(g.usedLetters, guess)
	return false, nil
}

func (g *Game) IsVictory() bool {
	for _, a := range g.correctGuesses {
		if !a {
			return false
		}
	}
	return true
}

func (g *Game) IsDefeat() bool {
	return g.numWrongGuesses >= maxWrongGuesses
}

func (g *Game) DrawMan() string {
	if g.numWrongGuesses >= len(boards) {
		return boards[len(boards)-1]
	}
	return boards[g.numWrongGuesses]
}

func (g *Game) GetGuessedWord() string {
	answer := strings.ToUpper(g.answer)
	word := make([]byte, 0, len(answer)*2)
	for i := range answer {
		if g.correctGuesses[i] {
			word = append(word, answer[i])
		} else {
			word = append(word, '_')
		}
		word = append(word, ' ')
	}
	return string(word)
}

func (g *Game) GetUsedLetters() string {
	letters := make([]byte, 0, len(g.usedLetters)*6)
	for _, l := range g.usedLetters {
		letters = append(letters, '~', '~', l, '~', '~', ' ')
	}
	return strings.ToUpper(string(letters))
}

func (g *Game) GetAnswer() string {
	return strings.ToUpper(g.answer)
}
