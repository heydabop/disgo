package hangman

import (
	"bytes"
	"errors"
	mrand "math/rand"
	"strings"
	"time"
)

var maxWrongGuesses = len(boards) - 1

type Game struct {
	answer string
	guessedLetters []bool
	numWrongGuesses int
	guesserID string
}

func NewGame(guesserID string) *Game {
	rand := mrand.New(mrand.NewSource(time.Now().UnixNano()))
	answer := wordlist[rand.Intn(len(wordlist))]
	return &Game{
		answer: answer,
		guessedLetters: make([]bool, len(answer)),
		numWrongGuesses: 0,
		guesserID: guesserID,
	}
}

func (g *Game) Guess(guesserID string, guess byte) (bool, error) {
	guess = bytes.ToLower([]byte{guess})[0]
	if (guesserID != g.guesserID) {
		return false, errors.New("You can't guess unless you started the game.")
	}
	correctGuess := false
	for i := range g.answer {
		if g.answer[i] == guess {
			correctGuess = true
			g.guessedLetters[i] = true
		}
	}
	if correctGuess {
		return true, nil
	}
	g.numWrongGuesses++
	return false, nil
}

func (g *Game) IsVictory() bool {
	for _, a := range g.guessedLetters {
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
	if (g.numWrongGuesses >= len(boards)) {
		return boards[len(boards) - 1]
	}
	return boards[g.numWrongGuesses]
}

func (g *Game) GetGuessedWord() string {
	answer := strings.ToUpper(g.answer)
	word := make([]byte, 0, len(answer) * 2)
	for i := range answer {
		if (g.guessedLetters[i]) {
			word = append(word, answer[i])
		} else {
			word = append(word, '_')
		}
		word = append(word, ' ')
	}
	return string(word)
}

func (g *Game) GetAnswer() string {
	return strings.ToUpper(g.answer)
}
