package main

//lint:file-ignore ST1005 Error strings can be capitalizaed, they're displayed to users

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	imageColor "image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io/ioutil"
	"math"
	"math/big"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/dgvoice"
	"github.com/bwmarrin/discordgo"
	"github.com/gyuho/goling/similar"
	"github.com/heydabop/disgo/climacell"
	"github.com/heydabop/disgo/darksky"
	"github.com/heydabop/disgo/google"
	"github.com/heydabop/disgo/hangman"
	"github.com/heydabop/disgo/markov"
	_ "github.com/lib/pq"
	"github.com/nfnt/resize"
	uuid "github.com/satori/go.uuid"
)

type commandFunc func(*discordgo.Session, string, string, string, string, []string) (string, error)

type stringFloatPair struct {
	AuthorID  string
	AvgLength float64
}

type stringFloatPairs []stringFloatPair

func (u stringFloatPairs) Len() int {
	return len(u)
}
func (u stringFloatPairs) Less(i, j int) bool {
	return u[i].AvgLength-u[j].AvgLength > 0
}
func (u stringFloatPairs) Swap(i, j int) {
	u[i], u[j] = u[j], u[i]
}

type userBet struct {
	UserID         string
	WinningNumbers []int
	Payout         float64
	Bet            float64
}

type shippoTrack struct {
	Carrier        string `json:"carrier"`
	TrackingNumber string `json:"tracking_number"`
	ETA            string `json:"eta"`
	TrackingStatus struct {
		Status        string    `json:"status"`
		StatusDetails string    `json:"status_details"`
		StatusDate    time.Time `json:"status_date"`
	} `json:"tracking_status"`
}

const (
	discordEpoch = 1420070400000
)

var (
	currentGame                               string
	currentVoiceSessions                      = make(map[string]*discordgo.VoiceConnection)
	currentVoiceChans                         = make(map[string]chan bool)
	diceRegex                                 = regexp.MustCompile(`(?i)(?:(\d+)\s*d\s*)?(\d+)(?:\s*([+-])\s*(\d+))?`)
	gamelist                                  []string
	lastKappa                                 = make(map[string]time.Time)
	lastMessagesByAuthor, lastCommandMessages = make(map[string]discordgo.Message), make(map[string]discordgo.Message)
	lastMessagesByChannel                     = make(map[string][4]string)
	lastQuoteIDs                              = make(map[string]int64)
	hangmanGames                              = make(map[string]*hangman.Game)
	ignoredUserIDs                            = make(map[[2]string]time.Time)
	mutedUserIDs                              = make(map[[2]string]time.Time)
	ownUserID                                 string
	ownUserIDint                              uint64
	pointRegex                                = regexp.MustCompile(`^(-?\d+\.?\d*)[,\s]+(-?\d+\.?\d*)$`)
	rouletteIsRed                             = []bool{true, false, true, false, true, false, true, false, true, false, false, true, false, true, false, true, false, true, true, false, true, false, true, false, true, false, true, false, false, true, false, true, false, true, false, true}
	rouletteBets                              = make(map[string][]userBet)
	rouletteTableValues                       = [][]int{{1, 2, 3}, {4, 5, 6}, {7, 8, 9}, {10, 11, 12}, {13, 14, 15}, {16, 17, 18}, {19, 20, 21}, {22, 23, 24}, {25, 26, 27}, {28, 29, 30}, {31, 32, 33}, {34, 35, 36}}
	rouletteWheelSpinningPending              = make(map[string]bool)
	rouletteWheelSpinning                     = make(map[string]bool)
	rouletteWheelValues                       = []int{32, 15, 19, 4, 12, 2, 25, 17, 34, 6, 27, 13, 36, 11, 30, 8, 23, 10, 5, 24, 16, 33, 1, 20, 14, 31, 9, 22, 18, 29, 7, 28, 12, 35, 3, 26, 0}
	startTime                                 = time.Now()
	sqlClient                                 *sql.DB
	timeoutedUserIDs                          = make(map[string]time.Time)
	userIDRegex                               = regexp.MustCompile(`<@!?(\d+?)>`)
	userIDUpQuotes                            = make(map[string][]string)
	voiceMutex                                sync.Mutex
	voteTime                                  = make(map[string]time.Time)
	wasNicknamed                              = make(map[string]bool)
)

func stopPlayer(guildID string) {
	if stop, found := currentVoiceChans[guildID]; found {
		stop <- true
	}
}

func timeSinceStr(timeSince time.Duration) string {
	str := ""
	if timeSince <= 1*time.Second {
		str = "less than a second"
	} else if timeSince < 120*time.Second {
		str = fmt.Sprintf("%.f seconds", timeSince.Seconds())
	} else if timeSince < 120*time.Minute {
		str = fmt.Sprintf("%.f minutes", timeSince.Minutes())
	} else if timeSince < 48*time.Hour {
		str = fmt.Sprintf("%.f hours", timeSince.Hours())
	} else {
		str = fmt.Sprintf("%.f days", timeSince.Hours()/24)
	}
	return str
}

func getUsername(session *discordgo.Session, userID, guildID string) (string, error) {
	member, err := session.GuildMember(guildID, userID)
	if err == nil {
		if len(member.Nick) > 0 {
			return member.Nick, nil
		}
		if member.User != nil && len(member.User.Username) > 0 {
			return member.User.Username, nil
		}
	}
	user, err := session.User(userID)
	if err != nil {
		errStr := err.Error()
		commaIndex := strings.Index(errStr, ",")
		if commaIndex != -1 {
			jsonStr := errStr[commaIndex+1:]
			var dErr struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			}
			jErr := json.Unmarshal([]byte(jsonStr), &dErr)
			if jErr != nil {
				fmt.Println(jErr.Error())
				return "", nil
			}
		}
		return "`<UNKNOWN>`", nil
	}
	return user.Username, nil
}

func getMostSimilarUserID(session *discordgo.Session, chanID, username string) (string, error) {
	channel, err := session.State.Channel(chanID)
	if err != nil {
		return "", err
	}
	guild, err := session.State.Guild(channel.GuildID)
	if err != nil {
		return "", err
	}
	var similarUsers [][2]string
	lowerUsername := strings.ToLower(username)
	if guild.Members != nil {
		for _, member := range guild.Members {
			if user := member.User; user != nil {
				if lname := strings.ToLower(user.Username); strings.Contains(lname, lowerUsername) {
					similarUsers = append(similarUsers, [2]string{lname, user.ID})
				}
			}
			if lname := strings.ToLower(member.Nick); len(lname) > 0 && strings.Contains(lname, lowerUsername) {
				similarUsers = append(similarUsers, [2]string{lname, member.User.ID})
			}
		}
	}
	if len(similarUsers) == 1 {
		return similarUsers[0][1], nil
	}
	maxSim := 0.0
	maxUserID := ""
	usernameBytes := []byte(lowerUsername)
	for _, user := range similarUsers {
		sim := similar.Cosine([]byte(strings.ToLower(user[0])), usernameBytes)
		if sim > maxSim {
			maxSim = sim
			maxUserID = user[1]
		}
	}
	if maxUserID == "" {
		return "", errors.New("No user found")
	}
	return maxUserID, nil
}

func getGameTimesFromRows(rows *sql.Rows, limit int) (stringFloatPairs, time.Time, int, float64, error) {
	userGame := make(map[string]string)
	userTime := make(map[string]time.Time)
	gameTime := make(map[string]float64)
	firstTime := time.Now()
	for rows.Next() {
		var userID, game, presence string
		var currTime time.Time
		err := rows.Scan(&userID, &currTime, &game, &presence)
		if err != nil {
			return make(stringFloatPairs, 0), time.Now(), 0, 0, err
		}
		if presence == "offline" {
			game = ""
		}

		if currTime.Before(firstTime) {
			firstTime = currTime
		}
		lastGame, found := userGame[userID]
		if !found && len(game) >= 1 {
			userGame[userID] = game
			userTime[userID] = currTime
			continue
		}

		if lastGame == game {
			continue
		}
		lastTime := userTime[userID]
		gameTime[lastGame] += currTime.Sub(lastTime).Hours()

		if len(game) < 1 {
			delete(userGame, userID)
			delete(userTime, userID)
		} else {
			userGame[userID] = game
			userTime[userID] = currTime
		}
	}
	now := time.Now()
	for userID, game := range userGame {
		lastTime := userTime[userID]
		gameTime[game] += now.Sub(lastTime).Hours()
	}
	totalTime := float64(0)
	gameTimes := make(stringFloatPairs, 0)
	for game, time := range gameTime {
		gameTimes = append(gameTimes, stringFloatPair{game, time})
		totalTime += time
	}
	sort.Sort(&gameTimes)
	if limit > len(gameTimes) {
		limit = len(gameTimes)
	}
	gameTimes = gameTimes[:limit]
	longestGameLength := 0
	for _, game := range gameTimes {
		if len(game.AuthorID) > longestGameLength {
			longestGameLength = len(game.AuthorID)
		}
	}
	return gameTimes, firstTime, longestGameLength, totalTime, nil
}

func getBetSpaces(args []string, req int) ([]int, error) {
	spaces := make([]int, req)
	for i, arg := range args {
		space, err := strconv.Atoi(arg)
		if err != nil {
			return spaces, err
		}
		if space > 36 || space < 0 {
			return spaces, fmt.Errorf("Space %d isn't on board", space)
		}
		spaces[i] = space
	}
	return spaces, nil
}

func getBetDetails(guildID, authorID string, args []string, req int) (float64, []int, error) {
	bet, err := strconv.ParseFloat(args[0], 64)
	if err != nil {
		return -1, []int{}, err
	}
	if bet < 0.1 {
		return -1, []int{}, errors.New("Bet below minimum of 0.1")
	}
	if len(args[1:]) < req {
		return -1, []int{}, fmt.Errorf("Missing spaces(s) in bet; %d given, %d needed", len(args[1:]), req)
	}
	if len(args[1:]) > req {
		return -1, []int{}, fmt.Errorf("Too many spaces for bet type; %d given, %d needed", len(args[1:]), req)
	}
	spaces, err := getBetSpaces(args[1:], req)
	if err != nil {
		return -1, []int{}, err
	}
	var money float64
	if err := sqlClient.QueryRow(`SELECT money FROM user_money WHERE guild_id = $1 AND user_id = $2`, guildID, authorID).Scan(&money); err != nil {
		if err == sql.ErrNoRows {
			money = 10
			if _, err := sqlClient.Exec(`INSERT INTO user_money(guild_id, user_id, money) VALUES ($1, $2, $3)`, guildID, authorID, money); err != nil {
				return -1, []int{}, err
			}
		} else {
			return -1, []int{}, err
		}
	}
	if money < bet {
		return -1, []int{}, errors.New("Like you can afford that.")
	}
	return bet, spaces, nil
}

func gambleChannelCheck(guildID, chanID string) error {
	if guildID == "98470233999675392" {
		if chanID != "190518994875318272" {
			return errors.New("")
		}
	}
	return nil
}

func getMarkovFilelist(name string) (files []string, err error) {
	cmd := exec.Command("find", "-iname", name+"_nolink")
	cmd.Dir = "/home/ross/markov/"
	out, err := cmd.Output()
	if err != nil {
		return
	}
	files = strings.Fields(string(out))
	return
}

func getShippoTrack(carrier, trackingNum string) (*shippoTrack, error) {
	client := http.Client{}
	req, err := http.NewRequest(
		"GET",
		fmt.Sprintf("https://api.goshippo.com/tracks/%s/%s", strings.ToLower(carrier), trackingNum),
		nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", fmt.Sprintf("ShippoToken %s", shippoToken))
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, errors.New(res.Status)
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	var status shippoTrack
	if err := json.Unmarshal(body, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

func spam(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	filename := ""
	if len(args) < 1 {
		files, err := getMarkovFilelist("*")
		if err != nil {
			return "", err
		}
		for i := range files {
			files[i] = strings.Replace(files[i], "./", "", 1)
			files[i] = strings.Replace(files[i], "_nolink", "", 1)
		}
		sort.Strings(files)
		return strings.Join(files, ", "), nil
	}
	files, err := getMarkovFilelist(args[0])
	if err != nil {
		return "", err
	}
	if len(files) < 1 {
		files, err := getMarkovFilelist("*" + args[0] + "*")
		if err != nil {
			return "", err
		}
		switch len(files) {
		case 0:
			return "", errors.New("No logs found for " + args[0])
		case 1:
			filename = files[0]
		default:
			for i := range files {
				files[i] = strings.Replace(files[i], "./", "", 1)
				files[i] = strings.Replace(files[i], "_nolink", "", 1)
			}
			sort.Strings(files)
			return "Did you mean one of the following: " + strings.Join(files, ", "), nil
		}
	} else {
		filename = files[0]
	}
	cmd := exec.Command("/home/ross/markov/1-markov.out", "1")
	logs, err := os.Open("/home/ross/markov/" + filename)
	if err != nil {
		return "", err
	}
	cmd.Stdin = logs
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:\n%s", filename[2:len(filename)-7], strings.TrimSpace(string(out))), nil
}

func changeMoney(guildID, userID string, value float64) error {
	_, err := sqlClient.Exec(`UPDATE user_money SET money = money + $1 WHERE guild_id = $2 AND user_id = $3`, value, guildID, userID)
	return err
}

func soda(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	return spam(session, guildID, chanID, authorID, messageID, []string{"sodapoppin"})
}

func lirik(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	return spam(session, guildID, chanID, authorID, messageID, []string{"lirik"})
}

func forsen(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	return spam(session, guildID, chanID, authorID, messageID, []string{"forsenlol"})
}

func cwc(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	return spam(session, guildID, chanID, authorID, messageID, []string{"cwc2016"})
}

func vote(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string, inc int) (string, error) {
	if len(args) < 1 {
		return "", errors.New("No userID provided")
	}
	userMention := args[0]
	var userID string
	if match := userIDRegex.FindStringSubmatch(userMention); match != nil {
		userID = match[1]
	} else {
		return "", errors.New("No valid mention found")
	}
	_, err := session.GuildMember(guildID, userID)
	if err != nil {
		return "", err
	}
	if authorID != ownUserID {
		lastVoteTime, validTime := voteTime[authorID]
		if validTime && time.Since(lastVoteTime).Minutes() < 5+5*rand.Float64() {
			return "Slow down champ.", nil
		}
	}
	if authorID != ownUserID && authorID == userID && inc > 0 {
		_, err := vote(session, guildID, chanID, ownUserID, messageID, []string{"<@" + authorID + ">"}, -1)
		if err != nil {
			return "", err
		}
		voteTime[authorID] = time.Now()
		return "No.", nil
	}

	var lastVoterIDAgainstUser string
	var lastVoteTime time.Time
	if err := sqlClient.QueryRow(`SELECT voter_id, create_date FROM vote WHERE guild_id = $1 AND votee_id = $2 ORDER BY create_date DESC LIMIT 1`, guildID, authorID).Scan(&lastVoterIDAgainstUser, &lastVoteTime); err != nil {
		if err == sql.ErrNoRows {
			lastVoterIDAgainstUser = ""
		} else {
			return "", err
		}
	}
	if authorID != ownUserID && lastVoterIDAgainstUser == userID && time.Since(lastVoteTime).Hours() < 12 {
		return "Really?...", nil
	}
	var lastVoteeIDFromAuthor string
	if err := sqlClient.QueryRow(`SELECT votee_id, create_date FROM vote WHERE guild_id = $1 AND voter_id = $2 ORDER BY create_date DESC LIMIT 1`, guildID, authorID).Scan(&lastVoteeIDFromAuthor, &lastVoteTime); err != nil {
		if err == sql.ErrNoRows {
			lastVoteeIDFromAuthor = ""
		} else {
			return "", err
		}
	}
	if authorID != ownUserID && lastVoteeIDFromAuthor == userID && time.Since(lastVoteTime).Hours() < 12 {
		return "Really?...", nil
	}

	var karma int
	if err := sqlClient.QueryRow(`SELECT karma FROM user_karma WHERE guild_id = $1 AND user_id = $2`, guildID, userID).Scan(&karma); err != nil {
		if err == sql.ErrNoRows {
			karma = 0
			if _, err := sqlClient.Exec(`INSERT INTO user_karma(guild_id, user_id, karma) VALUES ($1, $2, $3)`, guildID, userID, karma); err != nil {
				return "", err
			}
		} else {
			return "", err
		}
	}

	karma += inc
	if _, err := sqlClient.Exec(`UPDATE user_karma SET karma = $1 WHERE guild_id = $2 AND user_id = $3`, karma, guildID, userID); err != nil {
		return "", err
	}
	voteTime[authorID] = time.Now()

	messageIDUnit, err := strconv.ParseUint(messageID, 10, 64)
	if err != nil {
		return "", err
	}
	isUpvote := false
	if inc > 0 {
		isUpvote = true
	}
	if _, err := sqlClient.Exec(`INSERT INTO vote(guild_id, message_id, voter_id, votee_id, is_upvote) values ($1, $2, $3, $4, $5)`,
		guildID, messageIDUnit, authorID, userID, isUpvote); err != nil {
		return "", err
	}
	return "", nil
}

func upvote(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	return vote(session, guildID, chanID, authorID, messageID, args, 1)
}

func downvote(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	return vote(session, guildID, chanID, authorID, messageID, args, -1)
}

func votes(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	var limit int
	if len(args) < 1 {
		limit = 5
	} else {
		var err error
		limit, err = strconv.Atoi(args[0])
		if err != nil || limit < 0 {
			return "", err
		}
	}
	rows, err := sqlClient.Query(`SELECT user_id, karma FROM user_karma WHERE guild_id = $1 ORDER BY karma DESC LIMIT $2`, guildID, limit)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var votes []int
	var users []string
	for rows.Next() {
		var userID string
		var karma int
		err := rows.Scan(&userID, &karma)
		if err != nil {
			return "", err
		}
		votes = append(votes, karma)
		users = append(users, userID)
	}
	finalString := ""
	for i, vote := range votes {
		username, err := getUsername(session, users[i], guildID)
		if err != nil {
			return "", err
		}
		finalString += fmt.Sprintf("%s — %d\n", username, vote)
	}
	return finalString, nil
}

func money(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	var limit int
	if len(args) < 1 {
		limit = 5
	} else {
		var err error
		limit, err = strconv.Atoi(args[0])
		if err != nil || limit < 0 {
			return "", err
		}
	}
	rows, err := sqlClient.Query(`SELECT user_id, money FROM user_money WHERE guild_id = $1 ORDER BY money DESC LIMIT $2`, guildID, limit)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var monies []float64
	var users []string
	for rows.Next() {
		var userID string
		var money float64
		err := rows.Scan(&userID, &money)
		if err != nil {
			return "", err
		}
		monies = append(monies, money)
		users = append(users, userID)
	}
	finalString := "(Those not listed have 10)\n"
	for i, money := range monies {
		username, err := getUsername(session, users[i], guildID)
		if err != nil {
			return "", err
		}
		finalString += fmt.Sprintf("%s — %.2f\n", username, money)
	}
	return finalString, nil
}

func roll(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	var max big.Int
	one := big.NewInt(1)
	dice := uint64(1)
	addend := int64(0)
	if len(args) < 1 {
		max.SetInt64(100)
	} else {
		if match := diceRegex.FindStringSubmatch(strings.Join(args, " ")); match != nil {
			var err error
			if len(match[1]) > 0 {
				dice, err = strconv.ParseUint(match[1], 10, 64)
				if err != nil {
					return "", err
				}
			}
			if _, success := max.SetString(match[2], 10); !success {
				return "", fmt.Errorf(`Unable to parse "%s"`, match[2])
			}
			if len(match) == 5 && match[4] != "" {
				if addend, err = strconv.ParseInt(match[4], 10, 64); err != nil {
					return "", err
				}
				if match[3] == "-" {
					addend = 0 - addend
				}
			}
		}
	}
	rolls := make([]string, dice)
	var result big.Int
	total := big.NewInt(0)
	for i := uint64(0); i < dice; i++ {
		result.Rand(rand.New(rand.NewSource(time.Now().UnixNano())), &max)
		result.Add(&result, one)
		total.Add(total, &result)
		rolls[i] = result.Text(10)
	}
	total.Add(total, big.NewInt(addend))
	var message = strings.Join(rolls, " ")
	if dice > 1 || addend != 0 {
		if addend > 0 {
			message += fmt.Sprintf(" + %d", addend)
		} else if addend < 0 {
			message += fmt.Sprintf(" - %d", 0-addend)
		}
		message += fmt.Sprintf(" = %s", total.Text(10))
	}
	return message, nil
}

func uptime(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	output, err := exec.Command("uptime").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func top(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	var limit int
	if len(args) < 1 {
		limit = 5
	} else {
		var err error
		limit, err = strconv.Atoi(args[0])
		if err != nil || limit < 0 {
			return "", err
		}
	}
	chanIDint, err := strconv.ParseUint(chanID, 10, 64)
	if err != nil {
		return "", err
	}
	rows, err := sqlClient.Query(`SELECT author_id, count(author_id) AS num_messages FROM message WHERE chan_id = $1 AND content NOT LIKE '/%' GROUP BY author_id ORDER BY count(author_id) DESC LIMIT $2`, chanIDint, limit)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var counts []int64
	var users []string
	for rows.Next() {
		var authorID string
		var numMessages int64
		err := rows.Scan(&authorID, &numMessages)
		if err != nil {
			return "", err
		}
		counts = append(counts, numMessages)
		users = append(users, authorID)
	}
	finalString := ""
	for i, count := range counts {
		username, err := getUsername(session, users[i], guildID)
		if err != nil {
			return "", err
		}
		finalString += fmt.Sprintf("%s — %d\n", username, count)
	}
	return finalString, nil
}

func topLength(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	var limit int
	if len(args) < 1 {
		limit = 5
	} else {
		var err error
		limit, err = strconv.Atoi(args[0])
		if err != nil || limit < 0 {
			return "", err
		}
	}
	chanIDint, err := strconv.ParseUint(chanID, 10, 64)
	if err != nil {
		return "", err
	}
	rows, err := sqlClient.Query(`SELECT author_id, content FROM message WHERE chan_id = $1 AND content NOT LIKE '/%' AND trim(content) != ''`, chanIDint)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	messagesPerUser := make(map[string]uint)
	wordsPerUser := make(map[string]uint)
	urlRegex := regexp.MustCompile(`^https?:\/\/.*?\/[^[:space:]]*?$`)
	for i := 0; rows.Next(); i++ {
		var authorID string
		var message string
		err := rows.Scan(&authorID, &message)
		if err != nil {
			return "", err
		}
		if urlRegex.MatchString(message) {
			continue
		}
		messagesPerUser[authorID]++
		wordsPerUser[authorID] += uint(len(strings.Fields(message)))
	}
	avgLengths := make(stringFloatPairs, 0)
	for userID, numMessages := range messagesPerUser {
		avgLengths = append(avgLengths, stringFloatPair{userID, float64(wordsPerUser[userID]) / float64(numMessages)})
	}
	sort.Sort(&avgLengths)
	finalString := ""
	for i, length := range avgLengths {
		if i >= limit {
			break
		}
		username, err := getUsername(session, length.AuthorID, guildID)
		if err != nil {
			return "", err
		}
		finalString += fmt.Sprintf("%s — %.2f\n", username, length.AvgLength)
	}
	return finalString, nil
}

func rename(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	if len(args) < 1 {
		return "", errors.New("No new username provided")
	}
	newUsername := strings.Join(args[0:], " ")
	var lockedMinutes int
	var lastChangeTime time.Time
	now := time.Now()
	if err := sqlClient.QueryRow(`SELECT create_date, locked_minutes FROM own_username WHERE guild_id = $1 ORDER BY create_date DESC LIMIT 1`, guildID).Scan(&lastChangeTime, &lockedMinutes); err != nil {
		if err == sql.ErrNoRows {
			lockedMinutes = 0
		} else {
			return "", err
		}
	}

	if lockedMinutes == 0 || now.After(lastChangeTime.Add(time.Duration(lockedMinutes)*time.Minute)) {
		wasNicknamed[guildID] = true
		if err := session.GuildMemberNickname(guildID, "@me/nick", newUsername); err != nil {
			wasNicknamed[guildID] = false
			return "", err
		}

		var authorKarma int
		if err := sqlClient.QueryRow(`SELECT karma FROM user_karma WHERE guild_id = $1 AND user_id = $2`, guildID, authorID).Scan(&authorKarma); err != nil {
			authorKarma = 0
		}
		newLockedMinutes := rand.Intn(30) + 45 + 10*authorKarma
		if newLockedMinutes < 30 {
			newLockedMinutes = 30
		}

		if _, err := sqlClient.Exec(`INSERT INTO own_username (author_id, username, locked_minutes, guild_id) values ($1, $2, $3, $4)`,
			authorID, newUsername, newLockedMinutes, guildID); err != nil {
			return "", err
		}
		username, err := getUsername(session, authorID, guildID)
		if err != nil {
			return "", err
		}
		if authorKarma > 0 {
			return fmt.Sprintf("%s's name change will last for an extra %d minutes thanks to their karma!", username, 10*authorKarma), nil
		} else if authorKarma < 0 {
			return fmt.Sprintf("%s's name change will last up to %d minutes less due to their karma...", username, -10*authorKarma), nil
		}
	} else {
		return "I'm not ready to change who I am.", nil
	}
	return "", nil
}

func lastseen(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	if len(args) < 1 {
		return "", errors.New("No username provided")
	}
	var userID string
	var err error
	if match := userIDRegex.FindStringSubmatch(args[0]); match != nil {
		userID = match[1]
	} else {
		userID, err = getMostSimilarUserID(session, chanID, strings.Join(args, " "))
		if err != nil {
			return "", err
		}
	}
	username, err := getUsername(session, userID, guildID)
	if err != nil {
		return "", err
	}
	guild, err := session.State.Guild(guildID)
	if err != nil {
		return "", err
	}
	online := false
	for _, presence := range guild.Presences {
		if presence.User != nil && presence.User.ID == userID {
			online = presence.Status == discordgo.StatusOnline
			break
		}
	}
	if online {
		return fmt.Sprintf("%s is currently online", username), nil
	}
	var lastOnline time.Time
	guildIDint, err := strconv.ParseUint(guild.ID, 10, 64)
	if err != nil {
		return "", err
	}
	userIDint, err := strconv.ParseUint(userID, 10, 64)
	if err != nil {
		return "", err
	}
	if err := sqlClient.QueryRow(`SELECT create_date FROM user_presence WHERE guild_id = $1 AND user_id = $2 AND presence = 'online' ORDER BY create_date DESC LIMIT 1`, guildIDint, userIDint).Scan(&lastOnline); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Sprintf("%s was last seen at least %.f days ago", username, time.Since(time.Date(2016, 4, 7, 1, 7, 0, 0, time.Local)).Hours()/24), nil
		}
		return "", err
	}
	var offline time.Time
	if err := sqlClient.QueryRow(`SELECT create_date FROM user_presence WHERE guild_id = $1 AND user_id = $2 AND presence != 'online' AND create_date > $3 ORDER BY create_date ASC LIMIT 1`, guildIDint, userIDint, lastOnline).Scan(&offline); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Sprintf("%s is currently online", username), nil
		}
		return "", err
	}
	lastSeenStr := timeSinceStr(time.Since(offline))
	return fmt.Sprintf("%s was last seen %s ago", username, lastSeenStr), nil
}

func deleteLastMessage(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	lastMessage, msgFound := lastMessagesByAuthor[authorID]
	lastCommandMessage, cmdFound := lastCommandMessages[authorID]
	if msgFound && cmdFound {
		session.ChannelMessageDelete(lastMessage.ChannelID, lastMessage.ID)
		session.ChannelMessageDelete(lastCommandMessage.ChannelID, lastCommandMessage.ID)
		session.ChannelMessageDelete(chanID, messageID)
	}
	return "", nil
}

func kickme(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	perm, err := session.State.UserChannelPermissions(ownUserID, chanID)
	if err != nil {
		return "", err
	}
	if perm&discordgo.PermissionKickMembers == discordgo.PermissionKickMembers {
		time.AfterFunc(time.Second*time.Duration(rand.Intn(6)+4), func() {
			err := session.GuildMemberDelete(guildID, authorID)
			if err != nil {
				fmt.Println("ERROR in kickme", err)
				session.ChannelMessageSend(chanID, "jk")
				return
			}
			inv, err := session.ChannelInviteCreate(
				chanID,
				discordgo.Invite{
					MaxAge:    600, //10 minutes
					MaxUses:   1,
					Temporary: false,
				})
			if err != nil {
				fmt.Println("ERROR in kickme", err)
				return
			}
			privChan, err := session.UserChannelCreate(authorID)
			if err != nil {
				fmt.Println("ERROR in kickme", err)
				return
			}
			time.Sleep(5 * time.Second)
			_, err = session.ChannelMessageSend(privChan.ID, fmt.Sprintf("https://discord.gg/%s", inv.Code))
			if err != nil {
				fmt.Println("ERROR in kickme", err)
				return
			}
		})
		return "See ya nerd.", nil
	}
	return "You wish.", nil
}

func spamuser(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string, markovOrder int) (string, error) {
	if len(args) < 1 {
		return "", errors.New("No username provided")
	}
	var userID string
	var err error
	if match := userIDRegex.FindStringSubmatch(args[0]); match != nil {
		userID = match[1]
	} else {
		userID, err = getMostSimilarUserID(session, chanID, strings.Join(args, " "))
		if err != nil {
			return "", err
		}
	}
	username, err := getUsername(session, userID, guildID)
	if err != nil {
		return "", err
	}
	realUserID, err := strconv.ParseUint(userID, 10, 64)
	if err != nil {
		return "", err
	}

	var chanIDs uint64Array
	guild, err := session.State.Guild(guildID)
	if err != nil {
		return "", err
	}
	for _, channel := range guild.Channels {
		intChanID, err := strconv.ParseUint(channel.ID, 10, 64)
		if err != nil {
			return "", err
		}
		chanIDs = append(chanIDs, intChanID)
	}

	rows, err := sqlClient.Query(`SELECT content FROM message WHERE chan_id = ANY($1) AND author_id = $2`, chanIDs, realUserID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var corpus []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return "", err
		}
		corpus = append(corpus, line)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}

	outStr := ""
	numRows := int64(1)
	for i := 0; i < 100 && numRows > 0; i++ {
		switch markovOrder {
		case 1:
			outStr = markov.GenFirstOrder(corpus)
		case 2:
			outStr = markov.GenSecondOrder(corpus)
		case 3:
			outStr = markov.GenThirdOrder(corpus)
		default:
			return "", fmt.Errorf("Unrecognized markov order: %d", markovOrder)
		}

		if err != nil {
			return "", err
		}
		if err := sqlClient.QueryRow(`SELECT count(id) FROM message WHERE content LIKE $1 AND author_id = $2`, fmt.Sprintf("%%%s%%", outStr), realUserID).Scan(&numRows); err != nil {
			return "", err
		}
	}
	var quoteID int64
	if err := sqlClient.QueryRow(`INSERT INTO discord_quote(chan_id, author_id, content, score, is_fresh) VALUES ($1, $2, $3, 0, $4) RETURNING id`, chanID, userID, outStr, numRows == 0).Scan(&quoteID); err != nil {
		fmt.Println("ERROR inserting into DiscordQuote ", err.Error())
	} else {
		lastQuoteIDs[chanID] = quoteID
		userIDUpQuotes[chanID] = make([]string, 0)
	}
	return fmt.Sprintf("%s: %s", username, outStr), nil
}

func spamuser1(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	return spamuser(session, guildID, chanID, authorID, messageID, args, 1)
}

func spamuser2(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	return spamuser(session, guildID, chanID, authorID, messageID, args, 2)
}

func spamuser3(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	return spamuser(session, guildID, chanID, authorID, messageID, args, 3)
}

func spamdiscord(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string, markovOrder int) (string, error) {
	var chanIDs uint64Array
	guild, err := session.State.Guild(guildID)
	if err != nil {
		return "", err
	}
	for _, channel := range guild.Channels {
		intChanID, err := strconv.ParseUint(channel.ID, 10, 64)
		if err != nil {
			return "", err
		}
		chanIDs = append(chanIDs, intChanID)
	}

	rows, err := sqlClient.Query(`SELECT content FROM message WHERE chan_id = ANY ($1) AND content != '' AND author_id != $2`, chanIDs, ownUserID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var corpus []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return "", err
		}
		corpus = append(corpus, line)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}

	outStr := ""
	numRows := int64(1)
	for i := 0; i < 100 && numRows > 0; i++ {
		switch markovOrder {
		case 1:
			outStr = markov.GenFirstOrder(corpus)
		case 2:
			outStr = markov.GenSecondOrder(corpus)
		case 3:
			outStr = markov.GenThirdOrder(corpus)
		default:
			return "", fmt.Errorf("Unrecognized markov order: %d", markovOrder)
		}

		if err != nil {
			return "", err
		}
		if err := sqlClient.QueryRow(`SELECT count(id) FROM message WHERE content LIKE $1 AND chan_id = ANY ($2)`, fmt.Sprintf("%%%s%%", outStr), chanIDs).Scan(&numRows); err != nil {
			return "", err
		}
	}
	var quoteID int64
	if err := sqlClient.QueryRow(`INSERT INTO discord_quote(chan_id, content, score, is_fresh) values ($1, $2, 0, $3) RETURNING id`, chanID, outStr, numRows == 0).Scan(&quoteID); err != nil {
		fmt.Println("ERROR inserting into DiscordQuote ", err.Error())
	} else {
		lastQuoteIDs[chanID] = quoteID
		userIDUpQuotes[chanID] = make([]string, 0)
	}
	return outStr, nil
}

func spamdiscord1(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	return spamdiscord(session, guildID, chanID, authorID, messageID, args, 1)
}

func spamdiscord2(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	return spamdiscord(session, guildID, chanID, authorID, messageID, args, 2)
}

func spamdiscord3(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	return spamdiscord(session, guildID, chanID, authorID, messageID, args, 3)
}

func maths(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	if len(args) < 1 {
		return "", errors.New("Can't do math without maths")
	}
	formula := strings.Join(args, " ")
	res, err := http.Get(fmt.Sprintf("http://api.wolframalpha.com/v2/query?input=%s&appid=%s&format=plaintext", url.QueryEscape(formula), url.QueryEscape(wolframAppID)))
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", errors.New(res.Status)
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	var response struct {
		Success bool `xml:"success,attr"`
		Error   bool `xml:"error,attr"`
		NumPods int  `xml:"numpods,attr"`
		Pods    []struct {
			Title     string `xml:"title,attr"`
			Error     bool   `xml:"error,attr"`
			Primary   *bool  `xml:"primary,attr"`
			Plaintext string `xml:"subpod>plaintext"`
		} `xml:"pod"`
	}
	err = xml.Unmarshal(body, &response)
	if err != nil {
		return "", err
	}
	if response.NumPods == len(response.Pods) && response.NumPods > 0 {
		for _, pod := range response.Pods {
			if pod.Primary != nil && *(pod.Primary) {
				return pod.Plaintext, nil
			}
		}
	}
	return "", errors.New("No suitable answer found")
}

func define(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	return maths(session, guildID, chanID, authorID, messageID, append([]string{"define"}, args...))
}

func cputemp(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	output, err := exec.Command("sensors", "coretemp-isa-0000").Output()
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(output), "\n")
	return fmt.Sprintf("```%s```", strings.Join(lines[2:], "\n")), nil
}

func ayy(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	return "lmao", nil
}

func ping(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	output, err := exec.Command("ping", "-qc3", "discordapp.com").Output()
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(output), "\n")
	return fmt.Sprintf("```%s```", strings.Join(lines[len(lines)-3:], "\n")), nil
}

func xd(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	return "PUCK FALMER", nil
}

func asuh(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	var userID, joinUserID string
	if len(args) > 0 {
		var err error
		if match := userIDRegex.FindStringSubmatch(args[0]); match != nil {
			userID = match[1]
		} else {
			userID, err = getMostSimilarUserID(session, chanID, strings.Join(args, " "))
			if err != nil {
				return "", err
			}
		}
	}
	if len(userID) > 0 {
		joinUserID = userID
	} else {
		joinUserID = authorID
	}
	voiceMutex.Lock()
	defer voiceMutex.Unlock()

	guild, err := session.State.Guild(guildID)
	if err != nil {
		return "", err
	}
	voiceChanID := ""
	for _, state := range guild.VoiceStates {
		if state.UserID == joinUserID {
			voiceChanID = state.ChannelID
			break
		}
	}
	if voiceChanID == "" {
		return "I can't find which voice channel you're in.", nil
	}

	if currentVoiceSessions[guildID] != nil {
		if currentVoiceSessions[guildID].ChannelID == voiceChanID && currentVoiceSessions[guildID].GuildID == guild.ID {
			return "", nil
		}
		stopPlayer(guildID)
		err = currentVoiceSessions[guildID].Disconnect()
		currentVoiceSessions[guildID] = nil
		if err != nil {
			return "", err
		}
		time.Sleep(300 * time.Millisecond)
	}

	currentVoiceSessions[guildID], err = session.ChannelVoiceJoin(guild.ID, voiceChanID, false, false)
	if err != nil {
		currentVoiceSessions[guildID] = nil
		return "", err
	}

	time.Sleep(1 * time.Second)
	session.ChannelMessageDelete(chanID, messageID)
	for i := 0; i < 10; i++ {
		if !currentVoiceSessions[guildID].Ready || currentVoiceSessions[guildID].OpusSend == nil {
			time.Sleep(1 * time.Second)
			if i == 9 {
				err := currentVoiceSessions[guildID].Disconnect()
				currentVoiceSessions[guildID] = nil
				if err != nil {
					fmt.Println("ERROR disconnecting from voice channel " + err.Error())
				}
				return "", nil
			}
			continue
		}
		suh := rand.Intn(77)
		currentVoiceChans[guildID] = make(chan bool)
		dgvoice.PlayAudioFile(currentVoiceSessions[guildID], fmt.Sprintf("suh/suh%d.mp3", suh), currentVoiceChans[guildID])
		break
	}

	time.Sleep(5 * time.Second)

	if rand.Intn(3) == 0 {
		dgvoice.PlayAudioFile(currentVoiceSessions[guildID], "goodbye.mp3", currentVoiceChans[guildID])
		time.Sleep(1 * time.Second)
	}
	stopPlayer(guildID)
	err = currentVoiceSessions[guildID].Disconnect()
	currentVoiceSessions[guildID] = nil
	if err != nil {
		fmt.Println("ERROR disconnecting from voice channel " + err.Error())
	}
	return "", nil
}

func upquote(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	lastQuoteID, found := lastQuoteIDs[chanID]
	if !found {
		return "I can't find what I spammed last.", nil
	}
	for _, userID := range userIDUpQuotes[chanID] {
		if userID == authorID {
			return "You've already upquoted my last spam", nil
		}
	}
	if _, err := sqlClient.Exec(`UPDATE discord_quote SET score = score + 1 WHERE id = $1`, lastQuoteID); err != nil {
		return "", err
	}
	userIDUpQuotes[chanID] = append(userIDUpQuotes[chanID], authorID)
	return "", nil
}

func topquote(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	var limit int
	if len(args) < 1 {
		limit = 5
	} else {
		var err error
		limit, err = strconv.Atoi(args[0])
		if err != nil || limit < 0 {
			return "", err
		}
	}
	rows, err := sqlClient.Query(`SELECT author_id, content, score FROM discord_quote WHERE chan_id = $1 AND score > 0 ORDER BY score DESC LIMIT $2`, chanID, limit)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	channel, err := session.State.Channel(chanID)
	if err != nil {
		return "", err
	}
	messages := make([]string, limit)
	var i int
	for i = 0; rows.Next(); i++ {
		var authorID sql.NullString
		var content string
		var score int
		err = rows.Scan(&authorID, &content, &score)
		if err != nil {
			return "", err
		}
		authorName := `#` + channel.Name
		if authorID.Valid {
			username, err := getUsername(session, authorID.String, guildID)
			if err != nil {
				return "", err
			}
			authorName = username
		}
		messages[i] = fmt.Sprintf("%s (%d): %s", authorName, score, content)
	}
	return strings.Join(messages[:i], "\n"), nil
}

func eightball(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	responses := []string{"It is certain", "It is decidedly so", "Without a doubt", "Yes, definitely", "You may rely on it", "As I see it, yes", "Most likely", "Outlook good", "Yes", "Signs point to yes", "Reply hazy try again", "Ask again later", "Better not tell you now", "Cannot predict now", "Concentrate and ask again", "Don't count on it", "My reply is no", "My sources say no", "Outlook not so good", "Very doubtful"}
	return responses[rand.Intn(len(responses))], nil
}

func wlist(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	var limit int
	if len(args) < 1 {
		limit = 5
	} else {
		var err error
		limit, err = strconv.Atoi(args[0])
		if err != nil || limit < 0 {
			return "", err
		}
	}
	chanIDint, err := strconv.ParseUint(chanID, 10, 64)
	if err != nil {
		return "", err
	}
	rows, err := sqlClient.Query(`SELECT author_id, content FROM message WHERE chan_id = $1 AND content NOT LIKE '/%'`, chanIDint)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	countMap := make(map[string]int64)
	for rows.Next() {
		var authorID, message string
		err := rows.Scan(&authorID, &message)
		if err != nil {
			return "", err
		}
		messageWords := strings.Fields(message)
		for i, word := range messageWords {
			_, found := wlWords[word]
			if found {
				countMap[authorID]++
				continue
			}
			if i+2 > len(messageWords) {
				continue
			}
			_, found = wlWords[strings.Join(messageWords[i:i+2], " ")]
			if found {
				countMap[authorID]++
				continue
			}
			if i+3 > len(messageWords) {
				continue
			}
			_, found = wlWords[strings.Join(messageWords[i:i+3], " ")]
			if found {
				countMap[authorID]++
				continue
			}
			if i+4 > len(messageWords) {
				continue
			}
			_, found = wlWords[strings.Join(messageWords[i:i+4], " ")]
			if found {
				countMap[authorID]++
				continue
			}
		}
	}
	var counts stringFloatPairs
	for authorID, score := range countMap {
		var numMessages int64
		chanIDint, err := strconv.ParseUint(chanID, 10, 64)
		if err != nil {
			return "", err
		}
		authorIDint, err := strconv.ParseUint(authorID, 10, 64)
		if err != nil {
			return "", err
		}
		if err := sqlClient.QueryRow(`SELECT count(id) FROM message WHERE chan_id = $1 AND author_id = $2 AND content NOT LIKE '/%'`, chanIDint, authorIDint).Scan(&numMessages); err != nil {
			return "", err
		}
		counts = append(counts, stringFloatPair{authorID, float64(score) / float64(numMessages)})
	}
	if len(counts) == 0 {
		return "You're all clean!", nil
	}
	sort.Sort(&counts)
	length := limit
	if len(counts) < limit {
		length = len(counts)
	}
	output := make([]string, length)
	for i := 0; i < length; i++ {
		username, err := getUsername(session, counts[i].AuthorID, guildID)
		if err != nil {
			return "", err
		}
		output[i] = fmt.Sprintf("%s — %.4f", username, counts[i].AvgLength)
	}
	return strings.Join(output, "\n"), nil
}

func remindme(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	arg := strings.Join(args, " ")
	fmt.Println(arg)
	atTimeRegex := regexp.MustCompile(`(?i)(?:at\s+)?(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}\s+[\+-]\d{4})\s+(?:to\s+)?(.*)`)
	inTimeRegex := regexp.MustCompile(`(?i)(?:in)?\s*(?:(?:(?:(\d+)\s+years?)|(?:(\d+)\s+months?)|(?:(\d+)\s+weeks?)|(?:(\d+)\s+days?)|(?:(\d+)\s+hours?)|(?:(\d+)\s+minutes?)|(?:(\d+)\s+seconds?))(?:\sand)?\s?)+(?:to)?\s+(.*)`)
	atMatch := atTimeRegex.FindStringSubmatch(arg)
	inMatch := inTimeRegex.FindStringSubmatch(arg)
	fmt.Printf("%#v\n", atMatch)
	fmt.Printf("%#v\n", inMatch)
	if atMatch == nil && inMatch == nil {
		return "What?", nil
	}
	content := ""
	now := time.Now()
	var remindTime time.Time
	var err error
	if atMatch != nil {
		remindTime, err = time.Parse(`2006-01-02 15:04:05 -0700`, atMatch[1])
		if err != nil {
			return "", err
		}
		content = atMatch[2]
	} else {
		content = inMatch[8]
		var years, months, weeks, days int
		var hours, minutes, seconds int64
		var err error
		if len(inMatch[1]) > 0 {
			years, err = strconv.Atoi(inMatch[1])
			if err != nil {
				return "", err
			}
		}
		if len(inMatch[2]) > 0 {
			months, err = strconv.Atoi(inMatch[2])
			if err != nil {
				return "", err
			}
		}
		if len(inMatch[3]) > 0 {
			weeks, err = strconv.Atoi(inMatch[3])
			if err != nil {
				return "", err
			}
		}
		if len(inMatch[4]) > 0 {
			days, err = strconv.Atoi(inMatch[4])
			if err != nil {
				return "", err
			}
		}
		if len(inMatch[5]) > 0 {
			hours, err = strconv.ParseInt(inMatch[5], 10, 64)
			if err != nil {
				return "", err
			}
		}
		if len(inMatch[6]) > 0 {
			minutes, err = strconv.ParseInt(inMatch[6], 10, 64)
			if err != nil {
				return "", err
			}
		}
		if len(inMatch[7]) > 0 {
			seconds, err = strconv.ParseInt(inMatch[7], 10, 64)
			if err != nil {
				return "", err
			}
		}
		fmt.Printf("%dy %dm %dw %dd %dh %dm %ds\n", years, months, weeks, days, hours, minutes, seconds)
		remindTime = now.AddDate(years, months, weeks*7+days).Add(time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second)
	}
	fmt.Println(remindTime.Format(time.RFC3339))
	if remindTime.Before(now) {
		responses := []string{"Sorry, I lost my Delorean.", "Hold on, gotta hit 88MPH first.", "Too late.", "I'm sorry Dave, I can't do that.", ":|", "Time is a one-way street."}
		return responses[rand.Intn(len(responses))], nil
	}
	if _, err := sqlClient.Exec(`INSERT INTO reminder (chan_id, author_id, send_time, content) VALUES ($1, $2, $3, $4)`, chanID, authorID, remindTime.In(time.FixedZone("UTC", 0)), content); err != nil {
		return "", err
	}
	time.AfterFunc(remindTime.Sub(now), func() { session.ChannelMessageSend(chanID, fmt.Sprintf("<@%s> %s", authorID, content)) })
	return fmt.Sprintf("👍 %s", remindTime.Format(time.RFC1123Z)), nil
}

func meme(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	var opID, link string
	chanIDint, err := strconv.ParseUint(chanID, 10, 64)
	if err != nil {
		return "", err
	}
	for {
		if err := sqlClient.QueryRow(`SELECT author_id, content FROM message WHERE chan_id = $1 AND (content LIKE 'http://%' OR content LIKE 'https://%') AND author_id != $2 ORDER BY random() LIMIT 1`, chanIDint, ownUserIDint).Scan(&opID, &link); err != nil {
			return "", err
		}
		res, err := http.Head(link)
		if err != nil {
			return "", err
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			res.Body.Close()
			continue
		}
		username, err := getUsername(session, opID, guildID)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s: %s", username, link), nil
	}
}

func bitrate(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	guildChans, err := session.GuildChannels(guildID)
	if err != nil {
		return "", err
	}
	var chanRates stringFloatPairs
	longestChanLength := 0
	for _, guildChan := range guildChans {
		if guildChan != nil && guildChan.Type == discordgo.ChannelTypeGuildVoice {
			chanRates = append(chanRates, stringFloatPair{guildChan.Name, float64(guildChan.Bitrate) / 1000})
			if len(guildChan.Name) > longestChanLength {
				longestChanLength = len(guildChan.Name)
			}
		}
	}
	sort.Sort(&chanRates)
	message := ""
	for _, chanRates := range chanRates {
		message += fmt.Sprintf("%"+strconv.Itoa(longestChanLength)+"s — %.2fkbps\n", chanRates.AuthorID, chanRates.AvgLength)
	}
	return fmt.Sprintf("```%s```", message), nil
}

func age(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	if len(args) < 1 {
		return "", errors.New("No username provided")
	}
	var userID string
	var err error
	if match := userIDRegex.FindStringSubmatch(args[0]); match != nil {
		userID = match[1]
	} else {
		userID, err = getMostSimilarUserID(session, chanID, strings.Join(args, " "))
		if err != nil {
			return "", err
		}
	}
	member, err := session.GuildMember(guildID, userID)
	if err != nil {
		return "", err
	}
	if member.User == nil {
		return "", errors.New("No user found")
	}
	timeJoined, err := member.JoinedAt.Parse()
	if err != nil {
		return "", err
	}
	timeSince := timeSinceStr(time.Since(timeJoined))
	return fmt.Sprintf("%s joined this server %s ago on %s", member.User.Username, timeSince, timeJoined.Format("Jan _2, 2006")), nil
}

func userage(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	if len(args) < 1 {
		return "", errors.New("No username provided")
	}
	var userID string
	var err error
	if match := userIDRegex.FindStringSubmatch(args[0]); match != nil {
		userID = match[1]
	} else {
		userID, err = getMostSimilarUserID(session, chanID, strings.Join(args, " "))
		if err != nil {
			return "", err
		}
	}
	username, err := getUsername(session, userID, guildID)
	if err != nil {
		return "", err
	}
	userIDint, err := strconv.ParseUint(userID, 10, 64)
	if err != nil {
		return "", err
	}
	creationTime := time.Unix(int64((userIDint>>22)+discordEpoch)/1000, 0)
	timeSince := timeSinceStr(time.Since(creationTime))
	return fmt.Sprintf("%s joined Discord %s ago", username, timeSince), nil
}

func lastUserMessage(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	if len(args) < 1 {
		return "", errors.New("No username provided")
	}
	var userID string
	var err error
	if match := userIDRegex.FindStringSubmatch(args[0]); match != nil {
		userID = match[1]
	} else {
		userID, err = getMostSimilarUserID(session, chanID, strings.Join(args, " "))
		if err != nil {
			return "", err
		}
	}
	member, err := session.State.Member(guildID, userID)
	if err != nil {
		return "", err
	}
	if member.User == nil {
		return "", errors.New("No user found")
	}
	var timeSent time.Time
	chanIDint, err := strconv.ParseUint(chanID, 10, 64)
	if err != nil {
		return "", err
	}
	userIDint, err := strconv.ParseUint(userID, 10, 64)
	if err != nil {
		return "", err
	}
	if err := sqlClient.QueryRow("SELECT create_date FROM message WHERE chan_id = $1 AND author_id = $2 ORDER BY create_date DESC LIMIT 1", chanIDint, userIDint).Scan(&timeSent); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Sprintf("I've never seen %s say anything.", member.User.Username), nil
		}
		return "", err
	}
	timeSince := timeSinceStr(time.Since(timeSent))
	return fmt.Sprintf("%s sent their last message %s ago", member.User.Username, timeSince), nil
}

func color(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	if len(args) < 1 {
		return "", errors.New("No color specificed")
	}
	hexColorRegex := regexp.MustCompile(`(?i)^#?([\dA-F]{8}|[\dA-F]{6}|[\dA-F]{3,4})$`)
	hexColorMatch := hexColorRegex.FindStringSubmatch(args[0])
	if hexColorMatch == nil {
		return "", errors.New("Invalid color")
	}
	color := hexColorMatch[1]
	if len(color) < 6 {
		color = ""
		for _, char := range hexColorMatch[1] {
			color += string(char) + string(char)
		}
	}
	hexParseRegex := regexp.MustCompile(`(?i)^([\dA-F]{2})?([\dA-F]{2})([\dA-F]{2})([\dA-F]{2})$`)
	hexParseMatch := hexParseRegex.FindStringSubmatch(color)
	if hexParseMatch == nil {
		return "", errors.New("Invalid color")
	}

	var alpha64, red64, blue64, green64 uint64
	var alpha, red, blue, green uint8
	alpha64, err := strconv.ParseUint(hexParseMatch[1], 16, 8)
	if err != nil {
		alpha = 255
	} else {
		alpha = uint8(alpha64)
	}
	red64, err = strconv.ParseUint(hexParseMatch[2], 16, 8)
	if err != nil {
		return "", errors.New("Error parsing red value")
	}
	green64, err = strconv.ParseUint(hexParseMatch[3], 16, 8)
	if err != nil {
		return "", errors.New("Error parsing green value")
	}
	blue64, err = strconv.ParseUint(hexParseMatch[4], 16, 8)
	if err != nil {
		return "", errors.New("Error parsing blue value")
	}
	red, green, blue = uint8(red64), uint8(green64), uint8(blue64)

	x, y := 500, 250
	nrgbaImage := image.NewNRGBA(image.Rectangle{image.Point{0, 0}, image.Point{x, y}})
	for i := 0; i < x; i++ {
		for j := 0; j < y; j++ {
			nrgbaImage.SetNRGBA(i, j, imageColor.NRGBA{red, green, blue, alpha})
		}
	}
	imageBuffer := bytes.NewBuffer(make([]byte, 0, x*y))
	png.Encode(imageBuffer, nrgbaImage)

	_, err = session.ChannelFileSend(chanID, color+".png", imageBuffer)
	if err != nil {
		return "", err
	}
	return "", nil
}

func playtime(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	var limit int
	var userID, username string
	var rows *sql.Rows
	guildIDint, err := strconv.ParseUint(guildID, 10, 64)
	if err != nil {
		return "", err
	}
	if len(args) < 1 {
		limit = 10
	} else {
		var err error
		limit, err = strconv.Atoi(args[0])
		if limit < 0 {
			return "", nil
		}
		if err != nil { //try user mention
			limit = 10
			if match := userIDRegex.FindStringSubmatch(args[0]); match != nil {
				userID = match[1]
			} else {
				userID, err = getMostSimilarUserID(session, chanID, strings.Join(args, " "))
				if err != nil {
					return "", err
				}
			}
			username, err = getUsername(session, userID, guildID)
			if err != nil {
				return "", err
			}
			userIDint, err := strconv.ParseUint(userID, 10, 64)
			if err != nil {
				return "", err
			}
			rows, err = sqlClient.Query(`SELECT user_id, create_date, game, presence FROM user_presence WHERE guild_id = $1 AND user_id = $2 ORDER BY create_date ASC`, guildIDint, userIDint)
			if err != nil {
				return "", err
			}
		}
	}
	if rows == nil {
		var err error
		rows, err = sqlClient.Query(`SELECT user_id, create_date, game, presence FROM user_presence WHERE guild_id = $1 AND user_id != $2 AND user_id != $3 ORDER BY create_date ASC`, guildIDint, ownUserIDint, musicBotID)
		if err != nil {
			return "", err
		}
	}
	defer rows.Close()

	gameTimes, firstTime, longestGameLength, totalTime, err := getGameTimesFromRows(rows, limit)
	if err != nil {
		return "", err
	}

	var message string
	if len(username) > 0 {
		message = fmt.Sprintf("%s since %s\n", username, firstTime.Format(time.RFC1123Z))
	} else {
		message = fmt.Sprintf("Since %s\n", firstTime.Format(time.RFC1123Z))
	}
	message += fmt.Sprintf("%"+strconv.Itoa(longestGameLength)+"s — %.2f\n", "All Games", totalTime)
	for _, gameTime := range gameTimes {
		message += fmt.Sprintf("%"+strconv.Itoa(longestGameLength)+"s — %.2f\n", gameTime.AuthorID, gameTime.AvgLength)
	}
	return fmt.Sprintf("```%s```", message), nil
}

func recentPlaytime(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	now := time.Now()

	inTimeRegex := regexp.MustCompile(`(?i)(?:(?:(?:(\d+)\s+years?)|(?:(\d+)\s+months?)|(?:(\d+)\s+weeks?)|(?:(\d+)\s+days?)|(?:(\d+)\s+hours?)|(?:(\d+)\s+minutes?)|(?:(\d+)\s+seconds?))\s?)+(?:\s*(.*))?`)
	match := inTimeRegex.FindStringSubmatch(strings.Join(args, " "))
	if match == nil {
		return "What?", nil
	}
	fmt.Printf("%#v\n", match)
	selectionArg := match[8]
	var err error
	var years, months, weeks, days int
	var hours, minutes, seconds int64
	years, err = strconv.Atoi(match[1])
	if err != nil {
		days = 0
	}
	months, err = strconv.Atoi(match[2])
	if err != nil {
		days = 0
	}
	weeks, err = strconv.Atoi(match[3])
	if err != nil {
		days = 0
	}
	days, err = strconv.Atoi(match[4])
	if err != nil {
		days = 0
	}
	hours, err = strconv.ParseInt(match[5], 10, 64)
	if err != nil {
		hours = 0
	}
	minutes, err = strconv.ParseInt(match[6], 10, 64)
	if err != nil {
		minutes = 0
	}
	seconds, err = strconv.ParseInt(match[7], 10, 64)
	if err != nil {
		seconds = 0
	}
	err = nil
	fmt.Printf("%dy %dm %dw %dd %dh %dm %ds\n", years, months, weeks, days, hours, minutes, seconds)
	startTime := now.AddDate(-years, -months, -weeks*7-days).Add(time.Duration(-hours)*time.Hour + time.Duration(-minutes)*time.Minute + time.Duration(-seconds)*time.Second)
	fmt.Println(startTime.Format(time.RFC3339))

	guildIDint, err := strconv.ParseUint(guildID, 10, 64)
	if err != nil {
		return "", err
	}
	limit := 10
	var userID, username string
	var rows *sql.Rows
	if len(strings.Fields(selectionArg)) >= 1 {
		var err error
		limit, err = strconv.Atoi(selectionArg)
		if limit < 0 {
			return "", nil
		}
		if err != nil { //try user mention
			err = nil
			limit = 10
			if match := userIDRegex.FindStringSubmatch(selectionArg); match != nil {
				userID = match[1]
			} else {
				userID, err = getMostSimilarUserID(session, chanID, selectionArg)
				if err != nil {
					return "", err
				}
			}
			username, err = getUsername(session, userID, guildID)
			if err != nil {
				return "", err
			}
			userIDint, err := strconv.ParseUint(userID, 10, 64)
			if err != nil {
				return "", err
			}
			rows, err = sqlClient.Query(`SELECT user_id, create_date, game, presence FROM user_presence WHERE guild_id = $1 AND user_id = $2 AND create_date > $3 ORDER BY create_date ASC`, guildIDint, userIDint, startTime)
			if err != nil {
				return "", err
			}
		}
	}
	if rows == nil {
		rows, err = sqlClient.Query(`SELECT user_id, create_date, game, presence FROM user_presence WHERE guild_id = $1 AND user_id != $2 AND user_id != $3 AND create_date > $4 ORDER BY create_date ASC`, guildIDint, ownUserIDint, musicBotID, startTime)
	}
	if err != nil {
		return "", err
	}
	defer rows.Close()

	gameTimes, _, longestGameLength, totalTime, err := getGameTimesFromRows(rows, limit)
	if err != nil {
		return "", err
	}

	var message string
	if len(username) > 0 {
		message = fmt.Sprintf("%s since %s\n", username, startTime.Format(time.RFC1123Z))
	} else {
		message = fmt.Sprintf("Since %s\n", startTime.Format(time.RFC1123Z))
	}
	message += fmt.Sprintf("%"+strconv.Itoa(longestGameLength)+"s — %.2f\n", "All Games", totalTime)
	for _, gameTime := range gameTimes {
		message += fmt.Sprintf("%"+strconv.Itoa(longestGameLength)+"s — %.2f\n", gameTime.AuthorID, gameTime.AvgLength)
	}
	return fmt.Sprintf("```%s```", message), nil
}

func activity(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	var rows *sql.Rows
	var err error
	var username string
	channel, err := session.State.Channel(chanID)
	if err != nil {
		return "", err
	}
	chanIDint, err := strconv.ParseUint(chanID, 10, 64)
	if err != nil {
		return "", err
	}
	if len(args) > 0 {
		var userID string
		if match := userIDRegex.FindStringSubmatch(args[0]); match != nil {
			userID = match[1]
		} else {
			userID, err = getMostSimilarUserID(session, chanID, strings.Join(args, " "))
			if err != nil {
				return "", err
			}
		}
		username, err = getUsername(session, userID, guildID)
		if err != nil {
			return "", err
		}
		var userIDint uint64
		userIDint, err = strconv.ParseUint(userID, 10, 64)
		if err != nil {
			return "", err
		}
		rows, err = sqlClient.Query(`SELECT create_date FROM message WHERE chan_id = $1 AND author_id = $2 ORDER BY create_date ASC`, chanIDint, userIDint)
	} else {
		rows, err = sqlClient.Query(`SELECT create_date FROM message WHERE chan_id = $1 AND author_id != $2 ORDER BY create_date ASC`, chanIDint, ownUserIDint)
	}
	if err != nil {
		return "", err
	}
	defer rows.Close()
	hourCount := make([]uint64, 24)
	var firstTime, msgTime time.Time
	if rows.Next() {
		err = rows.Scan(&firstTime)
		if err != nil {
			return "", err
		}
		firstTime = firstTime.Local()
		if err != nil {
			return "", err
		}
		hourCount[firstTime.Hour()]++
	}
	for rows.Next() {
		err = rows.Scan(&msgTime)
		if err != nil {
			return "", err
		}
		msgTime = msgTime.Local()
		if err != nil {
			return "", err
		}
		hourCount[msgTime.Hour()]++
	}

	datapoints := ""
	maxPerHour := uint64(0)
	for i := 0; i <= 23; i++ {
		if hourCount[i] > maxPerHour {
			maxPerHour = hourCount[i]
		}
		datapoints += fmt.Sprintf("%d %d\n", i, hourCount[i])
	}

	datapointsFile, err := ioutil.TempFile("", "disgo")
	if err != nil {
		return "", err
	}
	defer os.Remove(datapointsFile.Name())
	defer datapointsFile.Close()
	plotFile, err := ioutil.TempFile("", "disgo")
	if err != nil {
		return "", err
	}
	defer os.Remove(plotFile.Name())
	defer plotFile.Close()
	err = ioutil.WriteFile(datapointsFile.Name(), []byte(datapoints), os.ModeTemporary)
	if err != nil {
		return "", err
	}

	title := fmt.Sprintf("#%s since %s", channel.Name, firstTime.Format(time.RFC1123Z))
	if len(username) > 0 {
		title = fmt.Sprintf("%s in #%s since %s", username, channel.Name, firstTime.Format(time.RFC1123Z))
	}
	err = exec.Command("gnuplot", "-e", fmt.Sprintf(`set terminal png size 700,400; set out "%s"; set key off; set xlabel "Hour"; set ylabel "Messages"; set yrange [0:%d]; set xrange [-1:24]; set boxwidth 0.75; set style fill solid; set xtics nomirror; set title noenhanced "%s"; plot "%s" using 1:2:xtic(1) with boxes`, plotFile.Name(), uint64(math.Ceil(float64(maxPerHour)*1.1)), title, datapointsFile.Name())).Run()
	if err != nil {
		return "", err
	}
	_, err = session.ChannelFileSend(chanID, plotFile.Name()+".png", plotFile)
	if err != nil {
		return "", err
	}
	return "", nil
}

func activityDay(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	var rows *sql.Rows
	var err error
	var username string
	channel, err := session.State.Channel(chanID)
	if err != nil {
		return "", err
	}
	chanIDint, err := strconv.ParseUint(chanID, 10, 64)
	if err != nil {
		return "", err
	}
	if len(args) > 0 {
		var userID string
		if match := userIDRegex.FindStringSubmatch(args[0]); match != nil {
			userID = match[1]
		} else {
			userID, err = getMostSimilarUserID(session, chanID, strings.Join(args, " "))
			if err != nil {
				return "", err
			}
		}
		username, err = getUsername(session, userID, guildID)
		if err != nil {
			return "", err
		}
		var userIDint uint64
		userIDint, err = strconv.ParseUint(userID, 10, 64)
		if err != nil {
			return "", err
		}
		rows, err = sqlClient.Query(`SELECT create_date FROM message WHERE chan_id = $1 AND author_id = $2 ORDER BY create_date ASC`, chanIDint, userIDint)
	} else {
		rows, err = sqlClient.Query(`SELECT create_date FROM message WHERE chan_id = $1 AND author_id != $2 ORDER BY create_date ASC`, chanIDint, ownUserIDint)
	}
	if err != nil {
		return "", err
	}
	defer rows.Close()
	dayCount := make([]uint64, 7)
	var firstTime, msgTime time.Time
	if rows.Next() {
		err = rows.Scan(&firstTime)
		if err != nil {
			return "", err
		}
		firstTime = firstTime.Local()
		if err != nil {
			return "", err
		}
		dayCount[firstTime.Weekday()]++
	}
	for rows.Next() {
		err = rows.Scan(&msgTime)
		if err != nil {
			return "", err
		}
		msgTime = msgTime.Local()
		if err != nil {
			return "", err
		}
		dayCount[msgTime.Weekday()]++
	}

	datapoints := ""
	maxPerDay := uint64(0)
	for i := 0; i <= 6; i++ {
		if dayCount[i] > maxPerDay {
			maxPerDay = dayCount[i]
		}
		datapoints += fmt.Sprintf("%d %d\n", i, dayCount[i])
	}

	datapointsFile, err := ioutil.TempFile("", "disgo")
	if err != nil {
		return "", err
	}
	defer os.Remove(datapointsFile.Name())
	defer datapointsFile.Close()
	plotFile, err := ioutil.TempFile("", "disgo")
	if err != nil {
		return "", err
	}
	defer os.Remove(plotFile.Name())
	defer plotFile.Close()
	err = ioutil.WriteFile(datapointsFile.Name(), []byte(datapoints), os.ModeTemporary)
	if err != nil {
		return "", err
	}

	title := fmt.Sprintf("#%s since %s", channel.Name, firstTime.Format(time.RFC1123Z))
	if len(username) > 0 {
		title = fmt.Sprintf("%s in #%s since %s", username, channel.Name, firstTime.Format(time.RFC1123Z))
	}
	err = exec.Command("gnuplot", "-e", fmt.Sprintf(`set terminal png size 700,400; set out "%s"; set key off; set xlabel "Day of Week"; set ylabel "Messages"; set yrange [0:%d]; set xrange [-1:7]; set boxwidth 0.75; set style fill solid; set xtics nomirror; set title noenhanced "%s"; plot "%s" using 1:2:xtic(1) with boxes`, plotFile.Name(), uint64(math.Ceil(float64(maxPerDay)*1.1)), title, datapointsFile.Name())).Run()
	if err != nil {
		return "", err
	}
	_, err = session.ChannelFileSend(chanID, plotFile.Name()+".png", plotFile)
	if err != nil {
		return "", err
	}
	return "", nil
}

func botuptime(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	uptime := time.Since(startTime)
	days := "days"
	if math.Floor(uptime.Hours()/24) == 1 {
		days = "day"
	}
	return fmt.Sprintf("%.f %s %02d:%02d", math.Floor(uptime.Hours()/24), days, uint64(math.Floor(uptime.Hours()))%24, uint64(math.Floor(uptime.Minutes()))%60), nil
}

func nest(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	dateStr := time.Now().Format("20060102")
	cmd := exec.Command("/home/ross/.gocode/src/github.com/heydabop/nesttracking/graph/graph", dateStr)
	cmd.Dir = "/home/ross/.gocode/src/github.com/heydabop/nesttracking/graph/"
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s.png", nestlogRoot, dateStr), nil
}

func roulette(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	err := gambleChannelCheck(guildID, chanID)
	if err != nil {
		return "Please don't do that in here. Try <#190518994875318272>", nil
	}
	if rouletteWheelSpinningPending[guildID] {
		return "I'm working on it.", nil
	}
	rouletteWheelSpinningPending[guildID] = true
	defer func() {
		rouletteWheelSpinningPending[guildID] = false
	}()
	if rouletteWheelSpinning[guildID] {
		return "Wheel is already spinning, place a bet", nil
	}
	value := rouletteWheelValues[rand.Intn(36)]
	colorStr := "Black"
	if value != 0 && rouletteIsRed[value-1] {
		colorStr = "Red"
	}
	time.AfterFunc(40*time.Second, func() {
		session.ChannelTyping(chanID)
		time.Sleep(5 * time.Second)
		if value == 0 {
			session.ChannelMessageSend(chanID, "Landed on 0")
		} else {
			session.ChannelMessageSend(chanID, fmt.Sprintf("%s %d", colorStr, value))
		}
		winner := false
		for _, bet := range rouletteBets[guildID] {
			betWin := false
			for _, betSpace := range bet.WinningNumbers {
				if betSpace == value {
					winner, betWin = true, true
					session.ChannelMessageSend(chanID, fmt.Sprintf("<@%s> wins %.2f more asuh bux!", bet.UserID, bet.Payout*bet.Bet))
					err := changeMoney(guildID, bet.UserID, (bet.Payout+1)*bet.Bet)
					if err != nil {
						session.ChannelMessageSend(chanID, "⚠ `"+err.Error()+"`")
					}
					break
				}
			}
			if !betWin {
				err := changeMoney(guildID, ownUserID, bet.Bet)
				if err != nil {
					session.ChannelMessageSend(chanID, "⚠ `"+err.Error()+"`")
				}
			}
		}
		if len(rouletteBets[guildID]) > 0 && !winner {
			session.ChannelMessageSend(chanID, "Everyone loses!")
		}
		rouletteBets[guildID] = make([]userBet, 0)
		rouletteWheelSpinning[guildID] = false
	})
	rouletteWheelSpinning[guildID] = true
	return "Spinning...", nil
}

func bet(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	err := gambleChannelCheck(guildID, chanID)
	if err != nil {
		return "Please don't do that in here. Try <#190518994875318272>", nil
	}
	if len(args) < 2 {
		privateChannel, err := session.UserChannelCreate(authorID)
		if err != nil {
			return "", err
		}
		_, err = session.ChannelMessageSend(privateChannel.ID, `The following bet types are allowed
All of these are proceeded with /bet <amount>, amount being how many bux you bet >= 0.1
Inside
`+"```"+`Straight - single <number> - payout if ball lands on given number, 0-36 inclusive - /bet 0.5 single 13
Split - split <number> <number> - on 1 of the 2 adjacent numbers - /bet 0.7 split 16 17
Street - street <number> - on 1 of the numbers in same row as given number - /bet 0.4 street 13
Corner - corner <number> <number> <number> <number> - on one of 4 given adjacent numbers - /bet 1 corner 25 26 28 29
Six Line - six <number> <number> - on one of 6 numbers from adjacent rows in which the 2 given numbers lie - /bet 1.5 six 13 16
Trio - trio <number> <number> - on 0 and one of the pairs 1, 2 or 2, 3- /bet 1.2 trio 1 2`+"```"+`
Outside
`+"```"+`Low - low - on 1-18
High - high - on 19-36
Red - red - on red
Black - black - on black
Even - even - on even
Odd - odd - on odd
Dozen - dozen <1, 2, or 3> - on first(1-12), second(13-24), or third(25-36) dozen - /bet 0.6 dozen 2
Column - column <1, 2, or 3> - on the given first, second, or third column - /bet 2.2 column 2
Snake - snake - on 1, 5, 9, 12, 14, 16, 19, 23, 27, 30, 32, or 34`+"```")
		if err != nil {
			return "", err
		}
		_, err = session.ChannelMessageSend(privateChannel.ID, "```"+`
┏━━━━┯━━━━┯━━━━┓
┃r  1┃b  2┃r  3┃
┣━━━━┿━━━━┿━━━━┥
┃b  4┃r  5┃b  6┃
┣━━━━┿━━━━┿━━━━┥
┃r  7┃b  8┃r  9┃
┣━━━━┿━━━━┿━━━━┥
┃b 10┃b 11┃r 12┃
┣━━━━┿━━━━┿━━━━┥
┃b 13┃r 14┃b 15┃
┣━━━━┿━━━━┿━━━━┥
┃r 16┃b 17┃r 18┃
┣━━━━┿━━━━┿━━━━┥
┃r 19┃b 20┃r 21┃
┣━━━━┿━━━━┿━━━━┥
┃b 22┃r 23┃b 24┃
┣━━━━┿━━━━┿━━━━┥
┃r 25┃b 26┃r 27┃
┣━━━━┿━━━━┿━━━━┥
┃b 28┃b 29┃r 30┃
┣━━━━┿━━━━┿━━━━┥
┃b 31┃r 32┃b 33┃
┣━━━━┿━━━━┿━━━━┥
┃r 34┃b 35┃r 36┃
┗━━━━┷━━━━┷━━━━┛
`+"```")
		if err != nil {
			return "", err
		}
		return "", nil
	}
	if !rouletteWheelSpinning[guildID] {
		return "The wheel must be spinning to place a bet. Try /spin", nil
	}
	var bet float64
	var spaces []int
	betArgs := make([]string, len(args)-1)
	betArgs[0] = args[0]
	for i := 1; i < len(betArgs); i++ {
		betArgs[i] = args[i+1]
	}
	switch strings.ToLower(args[1]) {
	case "single":
		bet, spaces, err = getBetDetails(guildID, authorID, betArgs, 1)
		if err != nil {
			return "", err
		}
		rouletteBets[guildID] = append(rouletteBets[guildID], userBet{authorID, spaces, 35, bet})
	case "split":
		if len(args) < 3 {
			return "", errors.New("Missing number(s) in split bet")
		}
		bet, spaces, err = getBetDetails(guildID, authorID, betArgs, 2)
		if err != nil {
			return "", err
		}
		if (spaces[0] != spaces[1]) && (((spaces[0]-1)/3 == (spaces[1]-1)/3 && int(math.Abs(float64(spaces[1]-spaces[0]))) == 1) || int(math.Abs(float64(spaces[1]-spaces[0]))) == 3 || ((spaces[0] == 0 || spaces[1] == 0) && int(math.Abs(float64(spaces[1]-spaces[0]))) <= 3)) {
			rouletteBets[guildID] = append(rouletteBets[guildID], userBet{authorID, spaces, 17, bet})
		} else {
			return "", fmt.Errorf("Spaces %v aren't adjacent", spaces)
		}
	case "street":
		bet, spaces, err = getBetDetails(guildID, authorID, betArgs, 1)
		if err != nil {
			return "", err
		}
		if spaces[0] == 0 {
			return "", errors.New("A street bet on 0 isn't valid")
		}
		space := spaces[0]
		spaces = make([]int, 3)
	outer:
		for _, row := range rouletteTableValues {
			for _, tableSpace := range row {
				if tableSpace == space {
					spaces = row
					break outer
				}
			}
		}
		rouletteBets[guildID] = append(rouletteBets[guildID], userBet{authorID, spaces, 11, bet})
	case "corner":
		bet, spaces, err = getBetDetails(guildID, authorID, betArgs, 4)
		if err != nil {
			return "", err
		}
		for _, space := range spaces {
			if space == 0 {
				return "", errors.New("Can't corner bet on 0")
			}
		}
		if spaces[1]-spaces[0] != 1 || spaces[3]-spaces[2] != 1 || (spaces[0]-1)/3 != (spaces[1]-1)/3 || (spaces[2]-1)/3 != (spaces[3]-1)/3 || ((spaces[2]-1)/3)-((spaces[0]-1)/3) != 1 || ((spaces[3]-1)/3)-((spaces[1]-1)/3) != 1 {
			return "", fmt.Errorf("Spaces %v aren't all adjacent. Note that spaces should be entered in ascending order. 16 17 19 20 isn't treated the same as 19 20 16 17", spaces)
		}
		rouletteBets[guildID] = append(rouletteBets[guildID], userBet{authorID, spaces, 8, bet})
	case "six":
		bet, spaces, err = getBetDetails(guildID, authorID, betArgs, 2)
		if err != nil {
			return "", err
		}
		if spaces[0] == 0 || spaces[1] == 0 {
			return "", errors.New("Can't six line bet on 0")
		}
		if int(math.Abs(float64(((spaces[0]-1)/3)-((spaces[1]-1)/3)))) != 1 {
			return "", fmt.Errorf("%d and %d aren't in adjacent rows", spaces[0], spaces[1])
		}
		betSpaces := make([]int, 0, 6)
		for _, space := range spaces {
			for _, row := range rouletteTableValues {
				for _, tableSpace := range row {
					if tableSpace == space {
						betSpaces = append(betSpaces, row...)
					}
				}
			}
		}
		rouletteBets[guildID] = append(rouletteBets[guildID], userBet{authorID, betSpaces, 5, bet})
	case "trio":
		bet, spaces, err = getBetDetails(guildID, authorID, betArgs, 2)
		if err != nil {
			return "", err
		}
		if spaces[0] < 1 || spaces[0] > 3 || spaces[1] < 1 || spaces[1] > 3 || int(math.Abs(float64(spaces[0])-float64(spaces[1]))) != 1 {
			return "", errors.New("Trio bet is only valid with 1 and 2 or 2 and 3")
		}
		spaces = append(spaces, 0)
		rouletteBets[guildID] = append(rouletteBets[guildID], userBet{authorID, spaces, 11, bet})
	case "low":
		bet, _, err = getBetDetails(guildID, authorID, betArgs, 0)
		if err != nil {
			return "", err
		}
		betSpaces := make([]int, 18)
		for i := 0; i < 18; i++ {
			betSpaces[i] = i + 1
		}
		rouletteBets[guildID] = append(rouletteBets[guildID], userBet{authorID, betSpaces, 1, bet})
	case "high":
		bet, _, err = getBetDetails(guildID, authorID, betArgs, 0)
		if err != nil {
			return "", err
		}
		betSpaces := make([]int, 18)
		for i := 0; i < 18; i++ {
			betSpaces[i] = i + 19
		}
		rouletteBets[guildID] = append(rouletteBets[guildID], userBet{authorID, betSpaces, 1, bet})
	case "red":
		bet, _, err = getBetDetails(guildID, authorID, betArgs, 0)
		if err != nil {
			return "", err
		}
		var betSpaces []int
		for i, isRed := range rouletteIsRed {
			if isRed {
				betSpaces = append(betSpaces, i+1)
			}
		}
		rouletteBets[guildID] = append(rouletteBets[guildID], userBet{authorID, betSpaces, 1, bet})
	case "black":
		bet, _, err = getBetDetails(guildID, authorID, betArgs, 0)
		if err != nil {
			return "", err
		}
		var betSpaces []int
		for i, isRed := range rouletteIsRed {
			if !isRed {
				betSpaces = append(betSpaces, i+1)
			}
		}
		rouletteBets[guildID] = append(rouletteBets[guildID], userBet{authorID, betSpaces, 1, bet})
	case "even":
		bet, _, err = getBetDetails(guildID, authorID, betArgs, 0)
		if err != nil {
			return "", err
		}
		betSpaces := make([]int, 0, 18)
		for i := 1; i <= 36; i++ {
			if i%2 == 0 {
				betSpaces = append(betSpaces, i)
			}
		}
		rouletteBets[guildID] = append(rouletteBets[guildID], userBet{authorID, betSpaces, 1, bet})
	case "odd":
		bet, _, err = getBetDetails(guildID, authorID, betArgs, 0)
		if err != nil {
			return "", err
		}
		betSpaces := make([]int, 0, 18)
		for i := 1; i <= 36; i++ {
			if i%2 == 1 {
				betSpaces = append(betSpaces, i)
			}
		}
		rouletteBets[guildID] = append(rouletteBets[guildID], userBet{authorID, betSpaces, 1, bet})
	case "dozen":
		bet, spaces, err = getBetDetails(guildID, authorID, betArgs, 1)
		if err != nil {
			return "", err
		}
		if spaces[0] != 1 && spaces[0] != 2 && spaces[0] != 3 {
			return "", errors.New("Dozen must be 1, 2, or 3 for first, second, or third dozen")
		}
		betSpaces := make([]int, 0, 12)
		for i := 12 * (spaces[0] - 1); i < 12*spaces[0]; i++ {
			betSpaces = append(betSpaces, i+1)
		}
		rouletteBets[guildID] = append(rouletteBets[guildID], userBet{authorID, betSpaces, 2, bet})
	case "column":
		bet, spaces, err = getBetDetails(guildID, authorID, betArgs, 1)
		if err != nil {
			return "", err
		}
		if spaces[0] != 1 && spaces[0] != 2 && spaces[0] != 3 {
			return "", errors.New("Column must be 1, 2, or 3")
		}
		betSpaces := make([]int, 12)
		for i, row := range rouletteTableValues {
			betSpaces[i] = row[spaces[0]-1]
		}
		rouletteBets[guildID] = append(rouletteBets[guildID], userBet{authorID, betSpaces, 2, bet})
	case "snake":
		bet, _, err = getBetDetails(guildID, authorID, betArgs, 0)
		if err != nil {
			return "", err
		}
		rouletteBets[guildID] = append(rouletteBets[guildID], userBet{authorID, []int{1, 5, 9, 12, 14, 16, 19, 23, 27, 30, 32, 34}, 2, bet})
	default:
		return "", errors.New("Unrecognized bet type")
	}
	err = changeMoney(guildID, authorID, -bet)
	if err != nil {
		return "", err
	}
	return "", nil
}

func give(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	if len(args) < 2 {
		return "", errors.New("/give <user> <amount>")
	}

	var giveeID string
	if match := userIDRegex.FindStringSubmatch(args[0]); match != nil {
		giveeID = match[1]
	} else {
		return "", errors.New("No valid mention found")
	}

	var amount float64
	var err error
	if amount, err = strconv.ParseFloat(args[1], 64); err != nil {
		return "", err
	}
	if amount < 0.1 {
		return "", errors.New("Given amount must be at least 0.1")
	}

	var money float64
	if err := sqlClient.QueryRow(`SELECT money FROM user_money WHERE guild_id = $1 AND user_id = $2`, guildID, authorID).Scan(&money); err != nil {
		if err == sql.ErrNoRows {
			money = 10
			if _, err := sqlClient.Exec(`INSERT INTO user_money(guild_id, user_id, money) VALUES ($1, $2, $3)`, guildID, authorID, money); err != nil {
				return "", err
			}
		} else {
			return "", err
		}
	}
	if money < amount {
		return "", errors.New("Like you can afford that.")
	}

	if err := changeMoney(guildID, authorID, -amount); err != nil {
		return "", err
	}
	if err := changeMoney(guildID, giveeID, amount); err != nil {
		return "", err
	}

	return fmt.Sprintf("Transaction complete. Don't spend it all in one place <@%s>...or do, whatever.", giveeID), nil
}

func topcommand(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	if len(args) < 1 {
		return "", errors.New("No command provided")
	}
	chanIDint, err := strconv.ParseUint(chanID, 10, 64)
	if err != nil {
		return "", err
	}
	rows, err := sqlClient.Query(`SELECT author_id, count(author_id) FROM message WHERE content LIKE $1 AND chan_id = $2 GROUP BY author_id ORDER BY count(author_id) DESC`, fmt.Sprintf(`/%s%%`, args[0]), chanIDint)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	message := ""
	for rows.Next() {
		var userID string
		var count int64
		err := rows.Scan(&userID, &count)
		if err != nil {
			return "", err
		}
		username, err := getUsername(session, userID, guildID)
		if err != nil {
			return "", err
		}
		message += fmt.Sprintf("%s — %d\n", username, count)
	}
	return message, nil
}

func gameactivity(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	guild, err := session.State.Guild(guildID)
	if err != nil {
		return "", err
	}
	guildIDint, err := strconv.ParseUint(guild.ID, 10, 64)
	if err != nil {
		return "", err
	}
	var rows *sql.Rows
	var enteredGame string
	if len(args) > 0 {
		enteredGame = strings.Join(args, " ")
		rows, err = sqlClient.Query(`SELECT user_id, create_date, game FROM user_presence  WHERE guild_id = $1 AND user_id != $2 AND (lower(game) = $3 OR game = '') ORDER BY create_date ASC`, guildIDint, ownUserIDint, strings.ToLower(enteredGame))
	} else {
		enteredGame = "All Games"
		rows, err = sqlClient.Query(`SELECT user_id, create_date, game FROM user_presence WHERE guild_id = $1 AND user_id != $2 ORDER BY create_date ASC`, guildIDint, ownUserIDint)
	}
	if err != nil {
		return "", err
	}
	defer rows.Close()
	hourCount := make([]uint64, 24)
	userStarted := make(map[string]time.Time)
	userGame := make(map[string]string)
	firstTime := time.Now()
	for rows.Next() {
		var userID, game string
		var currTime time.Time
		err := rows.Scan(&userID, &currTime, &game)
		if err != nil {
			return "", err
		}
		if currTime.Before(firstTime) {
			firstTime = currTime
		}

		lastTime, timeFound := userStarted[userID]
		lastGame, gameFound := userGame[userID]
		if !timeFound || (gameFound && len(lastGame) == 0) {
			userStarted[userID] = currTime
			userGame[userID] = game
			continue
		} else if game == lastGame {
			continue
		} else {
			if currTime.Hour() == lastTime.Hour() {
				hourCount[currTime.Hour()] += uint64(currTime.Minute() - lastTime.Minute())
			} else if currTime.Hour() > lastTime.Hour() {
				hourCount[lastTime.Hour()] += uint64(60 - lastTime.Minute())
				hourCount[currTime.Hour()] += uint64(currTime.Minute())
				for i := lastTime.Hour() + 1; i < currTime.Hour(); i++ {
					hourCount[i] += 60
				}
			} else {
				hourCount[lastTime.Hour()] += uint64(60 - lastTime.Minute())
				hourCount[currTime.Hour()] += uint64(currTime.Minute())
				for i := lastTime.Hour() + 1; i <= 23; i++ {
					hourCount[i] += 60
				}
				for i := 0; i < currTime.Hour(); i++ {
					hourCount[i] += 60
				}
			}
			userStarted[userID] = currTime
			userGame[userID] = game
		}
	}

	datapoints := ""
	maxPerHour := float64(0)
	for i := 0; i <= 23; i++ {
		gameHours := float64(hourCount[i]) / 60
		if gameHours > maxPerHour {
			maxPerHour = gameHours
		}
		datapoints += fmt.Sprintf("%d %.2f\n", i, gameHours)
	}
	if maxPerHour == 0 {
		return "", fmt.Errorf("No recorded playtime for %s\n", enteredGame)
	}

	datapointsFile, err := ioutil.TempFile("", "disgo")
	if err != nil {
		return "", err
	}
	defer os.Remove(datapointsFile.Name())
	defer datapointsFile.Close()
	plotFile, err := ioutil.TempFile("", "disgo")
	if err != nil {
		return "", err
	}
	defer os.Remove(plotFile.Name())
	defer plotFile.Close()
	err = ioutil.WriteFile(datapointsFile.Name(), []byte(datapoints), os.ModeTemporary)
	if err != nil {
		return "", err
	}

	title := fmt.Sprintf("%s in %s since %s", enteredGame, guild.Name, firstTime.Format(time.RFC1123Z))
	err = exec.Command("gnuplot", "-e", fmt.Sprintf(`set terminal png size 700,400; set out "%s"; set key off; set xlabel "Hour"; set ylabel "Game Hours"; set yrange [0:%d]; set xrange [-1:24]; set boxwidth 0.75; set style fill solid; set xtics nomirror; set title noenhanced "%s"; plot "%s" using 1:2:xtic(1) with boxes`, plotFile.Name(), uint64(math.Ceil(float64(maxPerHour)*1.1)), title, datapointsFile.Name())).Run()
	if err != nil {
		return "", err
	}
	_, err = session.ChannelFileSend(chanID, plotFile.Name()+".png", plotFile)
	if err != nil {
		return "", err
	}
	return "", nil
}

func invite(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	neededPermissions := discordgo.PermissionViewChannel |
		discordgo.PermissionSendMessages |
		discordgo.PermissionManageMessages |
		discordgo.PermissionReadMessageHistory |
		discordgo.PermissionEmbedLinks |
		discordgo.PermissionAttachFiles |
		discordgo.PermissionVoiceConnect |
		discordgo.PermissionVoiceSpeak |
		discordgo.PermissionVoiceMoveMembers |
		discordgo.PermissionCreateInstantInvite |
		discordgo.PermissionKickMembers |
		discordgo.PermissionManageChannels |
		0x4000000
	return fmt.Sprintf("https://discordapp.com/oauth2/authorize?client_id=%s&scope=bot&permissions=0x%X", appID, neededPermissions), nil
}

func updateAvatar(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	if authorID != adminID {
		return "", nil
	}

	avatar, err := os.Open("avatar.png")
	if err != nil {
		return "", err
	}
	defer avatar.Close()

	info, err := avatar.Stat()
	if err != nil {
		return "", err
	}
	buf := make([]byte, info.Size())

	reader := bufio.NewReader(avatar)
	reader.Read(buf)

	avatarBase64 := base64.StdEncoding.EncodeToString(buf)
	avatarBase64 = fmt.Sprintf("data:image/png;base64,%s", avatarBase64)

	self, err := session.User("@me")
	if err != nil {
		return "", err
	}

	_, err = session.UserUpdate("", "", self.Username, avatarBase64, "")
	if err != nil {
		return "", err
	}

	return "", nil
}

func lastPlayed(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	if len(args) < 1 {
		return "", errors.New("No username provided")
	}
	var userID string
	var err error
	if match := userIDRegex.FindStringSubmatch(args[0]); match != nil {
		userID = match[1]
	} else {
		userID, err = getMostSimilarUserID(session, chanID, strings.Join(args, " "))
		if err != nil {
			return "", err
		}
	}
	username, err := getUsername(session, userID, guildID)
	if err != nil {
		return "", err
	}
	guild, err := session.State.Guild(guildID)
	if err != nil {
		return "", err
	}
	var games []*discordgo.Activity
	for _, presence := range guild.Presences {
		if presence.User != nil && presence.User.ID == userID {
			games = presence.Activities
			break
		}
	}
	for _, g := range games {
		if g.Type == discordgo.ActivityTypeGame {
			return fmt.Sprintf("%s is currently playing %s", username, g.Name), nil
		}
	}
	guildIDint, err := strconv.ParseUint(guild.ID, 10, 64)
	if err != nil {
		return "", err
	}
	userIDint, err := strconv.ParseUint(userID, 10, 64)
	if err != nil {
		return "", err
	}
	var lastPlayedGame string
	var lastPlayed time.Time
	if err := sqlClient.QueryRow(`SELECT create_date, game FROM user_presence WHERE guild_id = $1 AND user_id = $2 AND game != '' ORDER BY create_date DESC LIMIT 1`, guildIDint, userIDint).Scan(&lastPlayed, &lastPlayedGame); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Sprintf("I've never seen %s play anything...", username), nil
		}
		return "", err
	}
	lastSeenStr := timeSinceStr(time.Since(lastPlayed))
	return fmt.Sprintf("%s last played %s %s ago", username, lastPlayedGame, lastSeenStr), nil
}

func whois(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	if len(args) < 1 {
		return "", nil
	}
	username, err := getUsername(session, args[0], guildID)
	if err != nil {
		return "", err
	}
	return username, nil
}

func permission(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	perm, err := session.State.UserChannelPermissions(ownUserID, chanID)
	if err != nil {
		return "", err
	}
	fmt.Printf("%X\n", perm)
	session.ChannelMessageDelete(chanID, messageID)
	return "", nil
}

func voicekick(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	member, err := session.State.Member(guildID, authorID)
	if err != nil {
		return "", err
	}
	authorized := false
	for _, roleID := range member.Roles {
		role, err := session.State.Role(guildID, roleID)
		if err != nil {
			return "", err
		}
		if role.Permissions&discordgo.PermissionAdministrator == discordgo.PermissionAdministrator || role.Permissions&discordgo.PermissionVoiceMoveMembers == discordgo.PermissionVoiceMoveMembers {
			authorized = true
			break
		}
	}
	if !authorized {
		return "", nil
	}
	if len(args) < 1 {
		return "", errors.New("No userID provided")
	}
	var userIDs []string
	for _, arg := range args {
		if match := userIDRegex.FindStringSubmatch(arg); match != nil {
			userIDs = append(userIDs, match[1])
		}
	}
	if len(userIDs) < 1 {
		return "", errors.New("No valid mentions found")
	}

	perm, err := session.State.UserChannelPermissions(ownUserID, chanID)
	if err != nil {
		return "", err
	}
	if perm&discordgo.PermissionManageChannels != discordgo.PermissionManageChannels || perm&discordgo.PermissionVoiceMoveMembers != discordgo.PermissionVoiceMoveMembers {
		return "I can't do that", nil
	}

	newChanName := fmt.Sprintf("kick-%04d", rand.Intn(10000))
	newChan, err := session.GuildChannelCreate(guildID, newChanName, discordgo.ChannelTypeGuildVoice)
	if err != nil {
		return "", err
	}
	for _, userID := range userIDs {
		err = session.GuildMemberMove(newChan.GuildID, userID, &newChan.ID)
		if err != nil {
			fmt.Println("ERROR in voicekick", err)
		}
	}
	_, err = session.ChannelDelete(newChan.ID)
	if err != nil {
		return "", err
	}
	return "", nil
}

func timeout(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	member, err := session.State.Member(guildID, authorID)
	if err != nil {
		return "", err
	}
	authorized := false
	for _, roleID := range member.Roles {
		role, err := session.State.Role(guildID, roleID)
		if err != nil {
			return "", err
		}
		if role.Permissions&discordgo.PermissionAdministrator == discordgo.PermissionAdministrator || role.Permissions&discordgo.PermissionVoiceMoveMembers == discordgo.PermissionVoiceMoveMembers {
			authorized = true
			break
		}
	}
	if !authorized {
		return "", nil
	}
	if len(args) < 1 {
		return "", errors.New("No userID provided")
	}
	var userIDs []string
	for _, arg := range args {
		if match := userIDRegex.FindStringSubmatch(arg); match != nil {
			userIDs = append(userIDs, match[1])
		}
	}
	if len(userIDs) < 1 {
		return "", errors.New("No valid mentions found")
	}

	perm, err := session.State.UserChannelPermissions(ownUserID, chanID)
	if err != nil {
		return "", err
	}
	if perm&discordgo.PermissionVoiceMoveMembers != discordgo.PermissionVoiceMoveMembers {
		return "I can't do that", nil
	}

	now := time.Now()
	timeout := timeoutChanID
	for _, userID := range userIDs {
		err = session.GuildMemberMove(timeoutGuildID, userID, &timeout)
		if err != nil {
			fmt.Println("ERROR in timeout", err)
		}
		timeoutedUserIDs[userID] = now
	}
	return "", nil
}

func topOnline(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	guildIDint, err := strconv.ParseUint(guildID, 10, 64)
	if err != nil {
		return "", err
	}
	rows, err := sqlClient.Query(`SELECT user_id, presence = 'online' AS online, create_date FROM user_presence WHERE guild_id = $1 AND (presence = 'online' OR presence = 'offline') AND create_date > '2016-08-30'`, guildIDint)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	userOnline := make(map[string]bool)
	var usersOnline, maxOnline int
	var maxTime time.Time
	var maxUserOnline []string
	for rows.Next() {
		var userID string
		var currTime time.Time
		var online bool
		if err := rows.Scan(&userID, &online, &currTime); err != nil {
			return "", err
		}
		if lastOnline, found := userOnline[userID]; found {
			if lastOnline == online {
				continue
			}
			if !lastOnline {
				usersOnline++
			} else {
				usersOnline--
			}
			userOnline[userID] = online
		} else if !found && online {
			usersOnline++
			userOnline[userID] = online
		}
		if usersOnline > maxOnline {
			maxOnline = usersOnline
			maxTime = currTime
			maxUserOnline = make([]string, 0, usersOnline)
			for onlineUserID, online := range userOnline {
				if online {
					maxUserOnline = append(maxUserOnline, onlineUserID)
				}
			}
			if len(maxUserOnline) != usersOnline {
				fmt.Println("oh no")
			}
		}
		if usersOnline < 0 {
			fmt.Println("uh oh")
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	onlineUsernames := make([]string, len(maxUserOnline))
	for i, userID := range maxUserOnline {
		username, err := getUsername(session, userID, guildID)
		if err != nil {
			return "", err
		}
		onlineUsernames[i] = username
	}
	return fmt.Sprintf("The following %d users were online on %s\n%s", maxOnline, maxTime.Format("Jan _2, 2006"), strings.Join(onlineUsernames, ", ")), nil
}

func ooer(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	arg := strings.Join(args, " ")
	var message []rune
	for _, r := range arg {
		message = append(message, r, rune(rand.Intn(0x70)+0x300), rune(rand.Intn(0x70)+0x300))
	}
	return string(message), nil
}

func superooer(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	message := strings.Join(args, " ")
	var err error
	for i := 0; i < 10; i++ {
		message, err = ooer(session, guildID, chanID, authorID, messageID, []string{message})
		if err != nil {
			return "", err
		}
	}
	return message, err
}

func serverAge(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	intGuildID, err := strconv.ParseUint(guildID, 10, 64)
	if err != nil {
		return "", err
	}
	creationTime := time.Unix(int64((intGuildID>>22)+discordEpoch)/1000, 0)

	return fmt.Sprintf("This server was created %s ago on %s", timeSinceStr(time.Since(creationTime)), creationTime.Format("Jan 02, 2006")), nil
}

func track(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	if len(args) < 2 {
		return "", errors.New("Missing carrier or tracking number")
	}
	carrier := strings.ToLower(args[0])
	trackingNum := args[1]
	status, err := getShippoTrack(carrier, trackingNum)
	if err != nil {
		return "", err
	}

	if status.TrackingStatus.Status != "DELIVERED" && status.TrackingStatus.Status != "FAILURE" {
		if _, err := sqlClient.Exec(`INSERT INTO shipment(carrier, tracking_number, chan_id, author_id) VALUES ($1, $2, $3, $4)`, status.Carrier, status.TrackingNumber, chanID, authorID); err != nil {
			fmt.Println("ERROR insert into Shipment", err)
		}
	}

	message := ""
	switch status.TrackingStatus.Status {
	case "UNKNOWN":
		message = fmt.Sprintf("Unable to find shipment with tracking number %s", trackingNum)
	case "TRANSIT":
		eta, err := time.Parse(time.RFC3339, status.ETA)
		if err != nil {
			fmt.Println("Error parsing shippo ETA", err)
			message = "Your shipment is in transit"
		} else {
			message = fmt.Sprintf("Your shipment is in transit. ETA: %s", eta.Format("Monday, Jan 02"))
		}
	case "DELIVERED":
		message = fmt.Sprintf("Your shipment was delivered at %s", status.TrackingStatus.StatusDate.Format(time.RFC1123Z))
	case "RETURNED":
		message = "Your shipment is being or has been returned to sender"
	case "FAILURE":
		message = fmt.Sprintf("There was an issue with delivery: %s", status.TrackingStatus.StatusDetails)
	default:
		return "", fmt.Errorf("Unrecognized status: %s", status.TrackingStatus.Status)
	}
	return message, nil
}

func greentext(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	numMessages := rand.Intn(5) + 3
	var rows *sql.Rows
	var err error
	chanIDint, err := strconv.ParseUint(chanID, 10, 64)
	if err != nil {
		return "", err
	}
	if len(args) > 0 {
		var userID string
		if match := userIDRegex.FindStringSubmatch(args[0]); match != nil {
			userID = match[1]
		} else {
			userID, err = getMostSimilarUserID(session, chanID, strings.Join(args, " "))
			if err != nil {
				return "", err
			}
		}
		var userIDint uint64
		userIDint, err = strconv.ParseUint(userID, 10, 64)
		if err != nil {
			return "", err
		}
		rows, err = sqlClient.Query(`SELECT content FROM message WHERE chan_id = $1 AND author_id = $2 AND length(content) > 0 AND content NOT LIKE '%
%' ORDER BY random() LIMIT $3`, chanIDint, userIDint, numMessages)
	} else {
		rows, err = sqlClient.Query(`SELECT content FROM message WHERE chan_id = $1 AND author_id != $2 AND length(content) > 0 AND content NOT LIKE '%
%' ORDER BY random() LIMIT $3`, chanIDint, ownUserIDint, numMessages)
	}
	if err != nil {
		return "", err
	}
	messages := make([]string, 0, numMessages)
	for rows.Next() {
		var message string
		if err := rows.Scan(&message); err != nil {
			return "", err
		}
		messages = append(messages, strings.Replace(message, `'`, `\'`, -1))
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	drawArg := ""
	for i, message := range messages {
		drawArg += fmt.Sprintf("text 0,%d '>%s'", 10*(i+1), message)
	}

	imageFile, err := ioutil.TempFile("", "disgoGreentext")
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	defer os.Remove(imageFile.Name())
	defer imageFile.Close()
	if err = exec.Command(
		"convert",
		"-size",
		fmt.Sprintf("400x%d", 10*(numMessages+1)),
		"xc:transparent",
		"-font",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"-pointsize",
		"10",
		"-fill",
		"#789922",
		"-stroke",
		"#789922",
		"-draw",
		drawArg,
		fmt.Sprintf("png:%s", imageFile.Name())).Run(); err != nil {
		return "", err
	}
	_, err = session.ChannelFileSend(chanID, "text.png", imageFile)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	return "", nil
}

func totalMessages(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	var messages uint64
	chanIDint, err := strconv.ParseUint(chanID, 10, 64)
	if err != nil {
		return "", err
	}
	if err := sqlClient.QueryRow(`SELECT count(id) FROM message WHERE chan_id = $1`, chanIDint).Scan(&messages); err != nil {
		return "", err
	}
	var firstTime time.Time
	if err := sqlClient.QueryRow(`SELECT create_date FROM message WHERE chan_id = $1 ORDER BY create_date ASC LIMIT 1`, chanIDint).Scan(&firstTime); err != nil {
		return "", err
	}
	timeSince := time.Since(firstTime)
	return fmt.Sprintf("%d messages have been sent in this channel since %s\nThat's %.2f per day or %.2f per hour", messages, firstTime.Format(time.RFC1123Z), float64(messages)/(timeSince.Hours()/24), float64(messages)/timeSince.Hours()), nil
}

func totalServers(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	userGuilds, err := session.UserGuilds(100, "", "")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("I am currently a member of %d servers", len(userGuilds)), nil
}

func source(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	return "https://github.com/heydabop/disgo", nil
}

func jpg(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	messages, err := session.ChannelMessages(chanID, 50, messageID, "", "")
	if err != nil {
		return "", nil
	}
	for _, message := range messages {
		var URL string
		if message.Attachments != nil && len(message.Attachments) > 0 {
			attachmentURL := message.Attachments[0].URL
			lastIndex := strings.LastIndex(attachmentURL, ".")
			if lastIndex == -1 || lastIndex == len(attachmentURL)-1 {
				continue
			}
			ext := strings.ToLower(attachmentURL[lastIndex+1:])
			if ext == "png" || ext == "jpg" || ext == "jpeg" || ext == "gif" {
				URL = attachmentURL
			}
		}
		if len(URL) < 1 {
			urlRegex := regexp.MustCompile(`(?i)https?:\/\/.+\.(?:jpg|png|jpeg|gif)`)
			if urlRegex.MatchString(message.Content) {
				URL = message.Content
			}
		}
		if len(URL) < 1 {
			continue
		}
		res, err := http.Get(URL)
		if err != nil {
			return "", err
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return "", errors.New(res.Status)
		}

		linkedImage, _, err := image.Decode(res.Body)
		if err != nil {
			return "", err
		}
		linkedImage = resize.Thumbnail(400, 400, linkedImage, resize.Bilinear)
		jpgImage := new(bytes.Buffer)
		options := jpeg.Options{Quality: 1}
		if err = jpeg.Encode(jpgImage, linkedImage, &options); err != nil {
			return "", err
		}

		if _, err = session.ChannelFileSend(chanID, "jpeg.jpg", jpgImage); err != nil {
			return "", err
		}
		return "", nil
	}
	return "I was unable to find a recently embedded image", nil
}

func giffy(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	messages, err := session.ChannelMessages(chanID, 50, messageID, "", "")
	if err != nil {
		return "", nil
	}
	for _, message := range messages {
		var URL string
		if message.Attachments != nil && len(message.Attachments) > 0 {
			attachmentURL := message.Attachments[0].URL
			lastIndex := strings.LastIndex(attachmentURL, ".")
			if lastIndex == -1 || lastIndex == len(attachmentURL)-1 {
				continue
			}
			ext := strings.ToLower(attachmentURL[lastIndex+1:])
			if ext == "png" || ext == "jpg" || ext == "jpeg" || ext == "gif" {
				URL = attachmentURL
			}
		}
		if len(URL) < 1 {
			urlRegex := regexp.MustCompile(`(?i)https?:\/\/.+\.(?:jpg|png|jpeg|gif)`)
			if urlRegex.MatchString(message.Content) {
				URL = message.Content
			}
		}
		if len(URL) < 1 {
			continue
		}
		res, err := http.Get(URL)
		if err != nil {
			return "", err
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return "", errors.New(res.Status)
		}

		linkedImage, _, err := image.Decode(res.Body)
		if err != nil {
			return "", err
		}
		linkedImage = resize.Thumbnail(400, 400, linkedImage, resize.Bilinear)
		gifImage := new(bytes.Buffer)
		options := gif.Options{NumColors: 128}
		if err = gif.Encode(gifImage, linkedImage, &options); err != nil {
			return "", err
		}

		if _, err = session.ChannelFileSend(chanID, "gif.gif", gifImage); err != nil {
			return "", err
		}
		return "", nil
	}
	return "I was unable to find a recently embedded image", nil
}

func ascii(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	return "```" + `
________________$$$$
______________$$____$$
______________$$____$$
______________$$____$$
______________$$____$$
______________$$____$$
__________$$$$$$____$$$$$$
________$$____$$____$$____$$$$
________$$____$$____$$____$$__$$
$$$$$$__$$____$$____$$____$$____$$
$$____$$$$________________$$____$$
$$______$$______________________$$
__$$____$$______________________$$
___$$$__$$______________________$$
____$$__________________________$$
_____$$$________________________$$
______$$______________________$$$
_______$$$____________________$$
________$$____________________$$
_________$$$________________$$$
__________$$________________$$
__________$$$$$$$$$$$$$$$$$$$$
` + "```", nil
}

func ignore(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	member, err := session.State.Member(guildID, authorID)
	if err != nil {
		return "", err
	}
	admin := false
	for _, roleID := range member.Roles {
		role, err := session.State.Role(guildID, roleID)
		if err != nil {
			return "", err
		}
		if role.Permissions&discordgo.PermissionAdministrator == discordgo.PermissionAdministrator {
			admin = true
			break
		}
	}
	if !admin {
		return "I don't have to listen to you.", nil
	}

	if len(args) < 1 {
		return "", errors.New("No user provided")
	}
	var userID string
	if match := userIDRegex.FindStringSubmatch(args[0]); match != nil {
		userID = match[1]
	} else {
		return "", errors.New("No valid mention found")
	}
	minutes := 5
	if len(args) > 1 {
		argMinutes, err := strconv.Atoi(args[1])
		if err != nil {
			return "", err
		}
		minutes = argMinutes
	}
	ignoredUserIDs[[2]string{guildID, userID}] = time.Now().Add(time.Duration(minutes) * time.Minute)

	return "", nil
}

func unignore(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	member, err := session.State.Member(guildID, authorID)
	if err != nil {
		return "", err
	}
	admin := false
	for _, roleID := range member.Roles {
		role, err := session.State.Role(guildID, roleID)
		if err != nil {
			return "", err
		}
		if role.Permissions&discordgo.PermissionAdministrator == discordgo.PermissionAdministrator {
			admin = true
			break
		}
	}
	if !admin {
		return "I don't have to listen to you.", nil
	}

	if len(args) < 1 {
		return "", errors.New("No user provided")
	}
	var userID string
	if match := userIDRegex.FindStringSubmatch(args[0]); match != nil {
		userID = match[1]
	} else {
		return "", errors.New("No valid mention found")
	}
	ignoredUserIDs[[2]string{guildID, userID}] = time.Now()

	return "", nil
}

func mute(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	member, err := session.State.Member(guildID, authorID)
	if err != nil {
		return "", err
	}
	admin := false
	for _, roleID := range member.Roles {
		role, err := session.State.Role(guildID, roleID)
		if err != nil {
			return "", err
		}
		if role.Permissions&discordgo.PermissionAdministrator == discordgo.PermissionAdministrator {
			admin = true
			break
		}
	}
	if !admin {
		return "I don't have to listen to you.", nil
	}

	if len(args) < 1 {
		return "", errors.New("No user provided")
	}
	var userID string
	if match := userIDRegex.FindStringSubmatch(args[0]); match != nil {
		userID = match[1]
	} else {
		return "", errors.New("No valid mention found")
	}
	minutes := 5
	if len(args) > 1 {
		argMinutes, err := strconv.Atoi(args[1])
		if err != nil {
			return "", err
		}
		minutes = argMinutes
	}
	mutedUserIDs[[2]string{guildID, userID}] = time.Now().Add(time.Duration(minutes) * time.Minute)

	return "", nil
}

func unmute(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	member, err := session.State.Member(guildID, authorID)
	if err != nil {
		return "", err
	}
	admin := false
	for _, roleID := range member.Roles {
		role, err := session.State.Role(guildID, roleID)
		if err != nil {
			return "", err
		}
		if role.Permissions&discordgo.PermissionAdministrator == discordgo.PermissionAdministrator {
			admin = true
			break
		}
	}
	if !admin {
		return "I don't have to listen to you.", nil
	}

	if len(args) < 1 {
		return "", errors.New("No user provided")
	}
	var userID string
	if match := userIDRegex.FindStringSubmatch(args[0]); match != nil {
		userID = match[1]
	} else {
		return "", errors.New("No valid mention found")
	}
	mutedUserIDs[[2]string{guildID, userID}] = time.Now()

	return "", nil
}

func dolphin(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	dolphinField := discordgo.MessageEmbedField{
		Name:  "Dolphin 5.0 Download",
		Value: "http://dl-mirror.dolphin-emu.org/5.0/dolphin-x64-5.0.exe",
	}
	configField := discordgo.MessageEmbedField{
		Name:  "Shared Config for Netplay",
		Value: "Coming Soon",
	}
	fields := []*discordgo.MessageEmbedField{&dolphinField, &configField}
	thumbnail := discordgo.MessageEmbedThumbnail{
		URL:    fmt.Sprintf("%s/dolphin.png", httpRoot),
		Width:  128,
		Height: 71,
	}
	embed := discordgo.MessageEmbed{
		Type:      "rich",
		Title:     "Dolphin Emulator Files",
		Thumbnail: &thumbnail,
		Color:     4641535,
		Fields:    fields,
	}
	if _, err := session.ChannelMessageSendEmbed(chanID, &embed); err != nil {
		return "", err
	}
	return "", nil
}

func fortune(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	out, err := exec.Command("fortune", "-as").Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func topEmoji(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	emojiRegex := regexp.MustCompile(`<:(.+?):(\d+)>`)
	var limit int
	if len(args) < 1 {
		limit = 10
	} else {
		var err error
		limit, err = strconv.Atoi(args[0])
		if err != nil || limit < 0 {
			return "", err
		}
	}
	chanIDint, err := strconv.ParseUint(chanID, 10, 64)
	if err != nil {
		return "", err
	}

	guild, err := session.State.Guild(guildID)
	if err != nil {
		return "", err
	}
	emojis := make(map[string]string, 50)
	for _, emoji := range guild.Emojis {
		emojis[emoji.ID] = emoji.Name
	}

	rows, err := sqlClient.Query(`SELECT content FROM message WHERE chan_id = $1 AND author_id <> $2`, chanIDint, ownUserIDint)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	counts := make(map[string]uint, 50)
	for rows.Next() {
		var message string
		err := rows.Scan(&message)
		if err != nil {
			return "", err
		}
		if matches := emojiRegex.FindAllStringSubmatch(message, -1); matches != nil {
			for _, match := range matches {
				if emojis[match[2]] == match[1] {
					counts[match[2]] = counts[match[2]] + 1
				}
			}
		}
	}
	type emojiCount struct {
		ID    string
		Count uint
	}
	sortableCounts := make([]emojiCount, 0, 50)
	for key, value := range counts {
		sortableCounts = append(sortableCounts, emojiCount{key, value})
	}
	sort.Slice(sortableCounts, func(i, j int) bool { return sortableCounts[i].Count > sortableCounts[j].Count })

	finalString := ""
	for i, count := range sortableCounts {
		if i > limit {
			break
		}
		finalString += fmt.Sprintf("<:%s:%s> — %d\n", emojis[count.ID], count.ID, count.Count)
	}
	return finalString, nil
}

func army(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	days := math.Ceil(time.Until(time.Date(2018, 6, 21, 0, 0, 0, 0, time.Local)).Hours() / 24)
	return fmt.Sprintf("%.0f days until June 21, 2018", days), nil
}

func willGrad(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	days := math.Ceil(time.Until(time.Date(2018, 5, 11, 0, 0, 0, 0, time.Local)).Hours() / 24)
	return fmt.Sprintf("%.0f days until May 11, 2018", days), nil
}

func grad(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	days := math.Ceil(time.Until(time.Date(2019, 5, 17, 0, 0, 0, 0, time.Local)).Hours() / 24)
	return fmt.Sprintf("%.0f days until May 17, 2019", days), nil
}

func courtney(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	sinceStart := time.Since(time.Date(2018, 5, 14, 8, 0, 0, 0, time.Local))
	untilEnd := time.Until(time.Date(2051, 4, 11, 17, 0, 0, 0, time.Local))
	return fmt.Sprintf("%.4f%% of the way to retirement", (sinceStart.Minutes()/untilEnd.Minutes())*100), nil
}

func ross(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	sinceStart := time.Since(time.Date(2016, 1, 11, 9, 30, 0, 0, time.Local))
	untilEnd := time.Until(time.Date(2048, 2, 28, 16, 0, 0, 0, time.Local))
	return fmt.Sprintf("%.4f%% of the way to retirement", (sinceStart.Minutes()/untilEnd.Minutes())*100), nil
}

func christmas(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	now := time.Now()
	if now.Month() != 12 || now.Day() > 25 {
		return "It's too early to be asking that", nil
	} else if now.Day() == 25 {
		return "Merry Christmas!", nil
	}
	days := math.Ceil(time.Until(time.Date(now.Year(), 12, 25, 0, 0, 0, 0, time.Local)).Hours() / 24)
	s := "s"
	if days == 1 {
		s = ""
	}
	return fmt.Sprintf("%.0f day%s until Christmas", days, s), nil
}

func hangmanCmd(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	if _, found := hangmanGames[chanID]; found {
		return "There's already a game going in here", nil
	}
	game := hangman.NewGame(authorID)
	hangmanGames[chanID] = game
	return fmt.Sprintf("```%s```\n`%s`", game.DrawMan(), game.GetGuessedWord()), nil
}

func guess(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	game, found := hangmanGames[chanID]
	if !found {
		return "No game is going, start one with /hangman", nil
	}
	if len(args) != 1 || len(args[0]) != 1 {
		return "", errors.New("/guess <letter>")
	}
	correct, err := game.Guess(authorID, args[0][0])
	if err != nil {
		return "", err
	}
	if correct {
		if game.IsVictory() {
			answer := game.GetAnswer()
			delete(hangmanGames, chanID)
			return fmt.Sprintf(":100: The word was %s", answer), nil
		}
		return fmt.Sprintf("`%s`\n%s", game.GetGuessedWord(), game.GetUsedLetters()), nil
	}
	if game.IsDefeat() {
		man := game.DrawMan()
		delete(hangmanGames, chanID)
		return fmt.Sprintf("```%s```\nGame over, you lose.", man), nil
	}
	return fmt.Sprintf("```%s```\n`%s`\n%s", game.DrawMan(), game.GetGuessedWord(), game.GetUsedLetters()), nil
}

func playing(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	if authorID != adminID || len(args) < 1 {
		return "", nil
	}
	if err := session.UpdateGameStatus(0, strings.Join(args[0:], " ")); err != nil {
		return "", err
	}
	return "", nil
}

func pee(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	lastPeeDate := time.Now().Add(-24 * time.Hour)
	if err := sqlClient.QueryRow(`SELECT create_date FROM pee_log WHERE user_id = $1 ORDER BY create_date DESC LIMIT 1`, authorID).Scan(&lastPeeDate); err != nil && err != sql.ErrNoRows {
		return "", err
	}
	if lastPeeDate.After(time.Now().Add(-30 * time.Minute)) {
		return "You *just* went, that doesn't count.", nil
	}
	if _, err := sqlClient.Exec(`INSERT INTO pee_log(user_id) VALUES ($1)`, authorID); err != nil {
		return "", err
	}
	responses := []string{
		"I'm proud of you.",
		"Well done!",
		"Keep it up.",
		"Clear and copious!",
		"What color was it?",
		":+1:",
		":ok_hand:",
		"Congrats",
		"Good job.",
		"same"}
	return responses[rand.Intn(len(responses))], nil
}

func peeCounter(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	counters := make(map[string]int)
	guild, err := session.State.Guild(guildID)
	if err != nil {
		return "", err
	}
	if guild.Members != nil {
		for _, member := range guild.Members {
			if user := member.User; user != nil {
				counters[user.ID] = 0
			}
		}
	}

	dayAgo := time.Now().Add(-24 * time.Hour)
	rows, err := sqlClient.Query(`SELECT user_id FROM pee_log WHERE create_date >= $1`, dayAgo)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return "", err
		}
		if _, found := counters[userID]; found {
			counters[userID]++
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}

	type usernameCount struct {
		Username string
		Count    int
	}

	longestUsernameLength := 0
	totalCount := 0
	var usernameCounters []usernameCount
	for userID, count := range counters {
		if count == 0 {
			continue
		}
		username, err := getUsername(session, userID, guildID)
		if err != nil {
			return "", err
		}
		totalCount += count
		usernameCounters = append(usernameCounters, usernameCount{username, count})
		if len(username) > longestUsernameLength {
			longestUsernameLength = len(username)
		}
	}

	sort.Slice(usernameCounters, func(i, j int) bool { return usernameCounters[i].Count > usernameCounters[j].Count })

	message := "Pee counts in the last 24 hours\n"
	message += fmt.Sprintf("%"+strconv.Itoa(longestUsernameLength)+"s - %d\n\n", "Total", totalCount)
	for _, uc := range usernameCounters {
		message += fmt.Sprintf("%"+strconv.Itoa(longestUsernameLength)+"s — %d\n", uc.Username, uc.Count)
	}
	return fmt.Sprintf("```%s```", message), nil
}

func poop(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	responses := []string{
		"tmi",
		"No thanks.",
		"That's disgusting.",
		"What is wrong with you?",
		"Stop it.",
		"No one wants to know that.",
		"🤮"}
	return responses[rand.Intn(len(responses))], nil
}

func sebbiTime(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	loc, err := time.LoadLocation("Europe/Copenhagen")
	if err != nil {
		return "", err
	}
	return time.Now().In(loc).Format("3:04 PM - Mon, Jan _2"), nil
}

func miroTime(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	loc, err := time.LoadLocation("Europe/Helsinki")
	if err != nil {
		return "", err
	}
	return time.Now().In(loc).Format("3:04 PM - Mon, Jan _2"), nil
}

func nielTime(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	loc, err := time.LoadLocation("Europe/Stockholm")
	if err != nil {
		return "", err
	}
	return time.Now().In(loc).Format("3:04 PM - Mon, Jan _2"), nil
}

func birdTime(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	loc, err := time.LoadLocation("Europe/Oslo")
	if err != nil {
		return "", err
	}
	return time.Now().In(loc).Format("3:04 PM - Mon, Jan _2"), nil
}

func realTime(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	loc, err := time.LoadLocation("America/Chicago")
	if err != nil {
		return "", err
	}
	return time.Now().In(loc).Format("15:04 - Mon, Jan _2"), nil
}

func euTime(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	loc1, err := time.LoadLocation("Europe/Lisbon")
	if err != nil {
		return "", err
	}
	loc2, err := time.LoadLocation("Asia/Nicosia")
	if err != nil {
		return "", err
	}
	now := time.Now()
	return fmt.Sprintf("%s - %s", now.In(loc1).Format("3:04 PM"), now.In(loc2).Format("3:04 PM")), nil
}

func weather(sesseion *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	var input string
	if len(args) == 0 {
		input = fmt.Sprintf("%s %s", darkSkyLat, darkSkyLng)
	} else {
		input = strings.Join(args, " ")
	}

	var lat, lng float64
	if match := pointRegex.FindStringSubmatch(input); match != nil {
		latStr := match[1]
		lngStr := match[2]
		var err error
		if lat, err = strconv.ParseFloat(latStr, 64); err != nil {
			return "", err
		}
		if lng, err = strconv.ParseFloat(lngStr, 64); err != nil {
			return "", err
		}
	} else {
		point, err := google.Geocode(input, mapsKey)
		if err != nil {
			return "", err
		}
		lat, lng = point.Lat, point.Lng
	}
	response, err := darksky.GetCurrent(lat, lng, darkSkySecret)
	if err != nil {
		return "", err
	}
	aqi, err := climacell.GetAQI(lat, lng, climaCellKey)
	var aqiStr string
	if err == nil {
		var health string
		if aqi < 51 {
			health = "Good"
		} else if aqi < 101 {
			health = "Moderate"
		} else if aqi < 151 {
			health = "Unhealthy for sensitive groups"
		} else if aqi < 201 {
			health = "Unhealthy"
		} else if aqi < 301 {
			health = "Very Unhealthy"
		} else {
			health = "Hazardous"
		}
		aqiStr = fmt.Sprintf("%d (%s)", aqi, health)
	} else {
		aqiStr = "--"
		fmt.Printf("Error getting ClimaCell AQI: %s", err)
	}
	return fmt.Sprintf(`temperature | %.0f °F (feels like %.0f °F)
conditions | %s
relative humidity | %.0f%% (dew point: %.0f °F)
wind | %.1f mph from %d° (gusts: %.1f mph)
uv index | %d
air quality index | %s
forecast | %s`,
		response.Currently.Temperature,
		response.Currently.ApparentTemperature,
		strings.ToLower(response.Currently.Summary),
		response.Currently.Humidity*100,
		response.Currently.DewPoint,
		response.Currently.WindSpeed,
		response.Currently.WindBearing,
		response.Currently.WindGust,
		response.Currently.UvIndex,
		aqiStr,
		strings.ToLower(strings.Trim(response.Hourly.Summary, "."))), nil
}

func help(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	privateChannel, err := session.UserChannelCreate(authorID)
	if err != nil {
		return "", err
	}
	_, err = session.ChannelMessageSend(privateChannel.ID, `**activity** - shows messages per hour over lifetime of channel
**age** [username] - displays how long [username] has been in this server
**asuh** - joins your voice channel
**ayy**
**bet** - place a roulette bet (type /bet for more help)
**bitrate** - shows voice channels and their bitrates
**botuptime** - shows time since bot last started
**color** [hex color code] - generates a solid image of given color
**cputemp** - displays CPU temperature
**cwc** - alias for /spam cwc2016
**delete** - deletes last message sent by bot (if you caused it)
**downvote** [@user] - downvotes user
**@[user]--** - downvotes user
**forsen** - alias for /spam forsenlol
**fortune** - get a "fortune"
**gameactivity** [game (optional)] - shows played hours per hour of <game> (or all games if none provided) over lifetime of channel
**gif** - looks for an embedded or linked image in the last 10 messages and reuploads it with modern GIF® compression`)
	if err != nil {
		return "", err
	}
	_, err = session.ChannelMessageSend(privateChannel.ID, `**greentext** - makes greentext with a couple messages from the channel's history
**jpg** - looks for an embedded or linked image in the last 10 messages and reuploads it with modern JPEG™ compression
**karma** [number (optional)] - displays top <number> users and their karma
**lastplayed** [username] - displays game last played by <username>
**lastseen** [username] - displays when <username> was last seen
**lastmessage** [username] - displays when <username> last sent a message
**lirik** - alias for /spam lirik
**math** [math stuff] - does math
**meme** - random meme from channel history
**messages** - displays how many messages have been sent in this channel
**money** [number (optional)] - displays top <number> users and their money
**ooer** [message] - Ǫ̧̩͟͜H̝̼ ̡̳͖͑̇M̔́Aͤ̓Ńͮ ̛̔ͯ͌ͪĮ̷̒̀͠ ͦ͋̐̾͡Ḁ̶͗ͪ͡Mͧͪ ̧ͩN̴̫̳̚͢Ǫ͈̬̫̏T̢̟̭͎͈ ̷̳̜̦͆G̵͛O̿́O̯͇̎̋͝D͖̈ ̼̰W͙̦̿͞͝I̛̮̊ͦ̚T̘͑H̨͎̲̑͢ ̢̗͍̟̽C̀ͯ͊̀͡O̷͈ͯ͌ͅM̓̓P̢̬̋̃͊U̜̱̓͡͞T̀̇Ě̷R̈̎ ̨̭ͭ̿͠P̳ͯͩ̎͟Ľ̳͏̨̩Ž̯ ͇̜Ť̤̻͖͜O̤̲҉̑ͯ ͤ͊H̢̼̿͆ͥḀ̢̢ͮ̊L̫͈̳̪̀P̶̯͆̾͟
**ping** - displays ping to discordapp.com
**playtime** [number (optional)] OR [username (options)] - shows up to <number> summated (probably incorrect) playtimes in hours of every game across all users, or top 10 games of <username>
**recentplaytime** [duration] [[number (optional)] OR [username (options)]] - same as playtime but with a duration (like remindme) before normal args, calculates only as far back as duration
**remindme**
	in [duration] to [x] - mentions user with <x> after <duration> (example: /remindme in 5 hours 10 minutes 3 seconds to order a pizza)
	at [time] to [x] - mentions user with <x> at <time> (example: /remindme at 2016-05-04 13:37:00 -0500 to make a clever xd facebook status)
**rename** [new username] - renames bot
**roll** [x]d[y] - "rolls" <x> dice with <y> sides, or rolls 1-100 if no dice are specified`)
	if err != nil {
		return "", err
	}
	_, err = session.ChannelMessageSend(privateChannel.ID, `**serverAge** - displays how long ago this server was created
**spam** [streamer (optional)] - generates a messages based on logs from <streamer>, shows all streamer logs if no streamer is specified
**spamdiscord** - generates a message based on logs from this discord channel
**spamdiscord2** - generates a message based on logs from this discord channel, less "creative" than spamdiscord but generally less nonsense
**spamuser** [username] - generates a message based on discord logs of <username>
**spamuser2** [username] - generates a message based on discord logs of <username>, less "creative" than spamuser but generally less nonsense
**spin** or **roulette** - spin roulette wheel
**soda** - alias for /spam sodapoppin
**source** - link to bot source code on github
**top** [number (optional)] - displays top <number> users sorted by messages sent
**topCommand** [command] - displays who has issued <command> most
**topEmoji** [number (optional)] - dispalys top <number> emojis sorted by times used
**topLength** [number (optional)] - dispalys top <number> users sorted by average words/message
**topOnline** - shows the maximum number of people that were ever simultaneously online
**topQuote** [number (optional)] - dispalys top <number> of "quotes" from bot spam, sorted by votes from /upquote
**track** [carrier] [tracking number] - displays current status of shipment and mentions you upon delivery
**uptime** - displays bot's server uptime and load
**upquote** - upvotes last statement generated by /spamuser or /spamdiscord
**userage** [username] - displays how long since [username] joined discord
**uq** - alias for /upquote
**upvote** [@user] - upvotes user
**@[user]++** - upvotes user
**votes** [number (optional)] - displays top <number> users and their karma
`+string([]byte{42, 42, 119, 97, 116, 99, 104, 108, 105, 115, 116, 42, 42, 32, 91, 110, 117, 109, 98, 101, 114, 32, 40, 111, 112, 116, 105, 111, 110, 97, 108, 41, 93, 32, 45, 32, 100, 105, 115, 112, 108, 97, 121, 115, 32, 116, 111, 112, 32, 60, 110, 117, 109, 98, 101, 114, 62, 32, 117, 115, 101, 114, 115, 32, 115, 111, 114, 116, 101, 100, 32, 98, 121, 32, 116, 101, 114, 114, 111, 114, 105, 115, 109, 32, 112, 101, 114, 32, 109, 101, 115, 115, 97, 103, 101})+`
**xd**
**zalgo** [message] - H̢͘͢È̛̛͡ ̴̛̛̀͏Ç̸O̕͝͏͏͡M̷̕E͘͘͡S̶̛`)
	if err != nil {
		return "", err
	}
	session.ChannelMessageDelete(chanID, messageID)
	return "", nil
}

func kappa(session *discordgo.Session, chanID, authorID, messageID string) {
	perm, err := session.State.UserChannelPermissions(ownUserID, chanID)
	if err != nil {
		return
	}
	if lastTime, found := lastKappa[authorID]; perm&discordgo.PermissionManageMessages == discordgo.PermissionManageMessages && (!found || time.Since(lastTime) > 30*time.Second) {
		image, err := os.Open("kappa.png")
		if err != nil {
			return
		}
		_, err = session.ChannelFileSend(chanID, "kappa.png", image)
		if err == nil {
			session.ChannelMessageDelete(chanID, messageID)
		}
	} else {
		session.ChannelMessageDelete(chanID, messageID)
	}
	lastKappa[authorID] = time.Now()
}

func speedtest(session *discordgo.Session, guildID, chanID, authorID, messageID string, args []string) (string, error) {
	if authorID != adminID {
		return "", nil
	}

	cmd := exec.Command("speedtest-cli", "--simple")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("```%s```", out), nil
}

func makeMessageCreate() func(*discordgo.Session, *discordgo.MessageCreate) {
	commandRegex := regexp.MustCompile(`^\/(.+)`)
	upvoteRegex := regexp.MustCompile(`(<@!?\d+?>)\s*\+\+`)
	downvoteRegex := regexp.MustCompile(`(<@!?\d+?>)\s*--`)
	meanRegex := regexp.MustCompile(`(?i)((fuc)|(shit)|(garbage)|(garbo)).*bot($|[[:space:]])`)
	botRegex := regexp.MustCompile(`(?i)(^|\s)(bot|robot)($|\s)`)
	questionRegex := regexp.MustCompile(`^<@!?` + ownUserID + `>.*\w+.*\?$`)
	inTheChatRegex := regexp.MustCompile(`(?i)can i get an?\s+(.*)\s+in the chat`)
	kappaRegex := regexp.MustCompile(`(?i)^\s*kappa\s*$`)
	//greenTextRegex := regexp.MustCompile(`(?i)^\s*>\s*([^:].+)$`)
	funcMap := map[string]commandFunc{
		"8ball":          commandFunc(eightball),
		"activity":       commandFunc(activity),
		"activityday":    commandFunc(activityDay),
		"actualtime":     commandFunc(realTime),
		"age":            commandFunc(age),
		"army":           commandFunc(army),
		"ascii":          commandFunc(ascii),
		"asuh":           commandFunc(asuh),
		"ayy":            commandFunc(ayy),
		"bet":            commandFunc(bet),
		"birdtime":       commandFunc(birdTime),
		"bitrate":        commandFunc(bitrate),
		"botuptime":      commandFunc(botuptime),
		"christmas":      commandFunc(christmas),
		"color":          commandFunc(color),
		"command":        commandFunc(help),
		"commands":       commandFunc(help),
		"courtney":       commandFunc(courtney),
		"cputemp":        commandFunc(cputemp),
		"cwc":            commandFunc(cwc),
		"define":         commandFunc(define),
		"delete":         commandFunc(deleteLastMessage),
		"dolphin":        commandFunc(dolphin),
		"downvote":       commandFunc(downvote),
		"eutime":         commandFunc(euTime),
		"forsen":         commandFunc(forsen),
		"fortune":        commandFunc(fortune),
		"g":              commandFunc(guess),
		"gameactivity":   commandFunc(gameactivity),
		"gif":            commandFunc(giffy),
		"give":           commandFunc(give),
		"goodtime":       commandFunc(realTime),
		"grad":           commandFunc(grad),
		"greentext":      commandFunc(greentext),
		"guess":          commandFunc(guess),
		"hangman":        commandFunc(hangmanCmd),
		"help":           commandFunc(help),
		"ignore":         commandFunc(ignore),
		"invite":         commandFunc(invite),
		"jpg":            commandFunc(jpg),
		"karma":          commandFunc(votes),
		"kickme":         commandFunc(kickme),
		"kms":            commandFunc(kickme),
		"lastmessage":    commandFunc(lastUserMessage),
		"lastplayed":     commandFunc(lastPlayed),
		"lastseen":       commandFunc(lastseen),
		"lirik":          commandFunc(lirik),
		"math":           commandFunc(maths),
		"meme":           commandFunc(meme),
		"messages":       commandFunc(totalMessages),
		"mirotime":       commandFunc(miroTime),
		"money":          commandFunc(money),
		"mute":           commandFunc(mute),
		"natime":         commandFunc(realTime),
		"nest":           commandFunc(nest),
		"nieltime":       commandFunc(nielTime),
		"ooer":           commandFunc(ooer),
		"pee":            commandFunc(pee),
		"peecounter":     commandFunc(peeCounter),
		"permission":     commandFunc(permission),
		"ping":           commandFunc(ping),
		"playing":        commandFunc(playing),
		"playtime":       commandFunc(playtime),
		"poo":            commandFunc(poop),
		"poop":           commandFunc(poop),
		"realtime":       commandFunc(realTime),
		"recentplaytime": commandFunc(recentPlaytime),
		"remindme":       commandFunc(remindme),
		"rename":         commandFunc(rename),
		"roll":           commandFunc(roll),
		"ross":           commandFunc(ross),
		"roulette":       commandFunc(roulette),
		"sebbitime":      commandFunc(sebbiTime),
		"serverage":      commandFunc(serverAge),
		"servers":        commandFunc(totalServers),
		"soda":           commandFunc(soda),
		"source":         commandFunc(source),
		"spam":           commandFunc(spam),
		"spamdiscord":    commandFunc(spamdiscord1),
		"spamdiscord2":   commandFunc(spamdiscord2),
		"spamdiscord3":   commandFunc(spamdiscord3),
		"spamuser":       commandFunc(spamuser1),
		"spamuser2":      commandFunc(spamuser2),
		"spamuser3":      commandFunc(spamuser3),
		"speedtest":      commandFunc(speedtest),
		"spin":           commandFunc(roulette),
		"superooer":      commandFunc(superooer),
		"timeout":        commandFunc(timeout),
		"top":            commandFunc(top),
		"topcommand":     commandFunc(topcommand),
		"topemoji":       commandFunc(topEmoji),
		"toplength":      commandFunc(topLength),
		"toponline":      commandFunc(topOnline),
		"topquote":       commandFunc(topquote),
		"track":          commandFunc(track),
		"twintime":       commandFunc(realTime),
		"unignore":       commandFunc(unignore),
		"unmute":         commandFunc(unmute),
		"updateavatar":   commandFunc(updateAvatar),
		"upquote":        commandFunc(upquote),
		"uptime":         commandFunc(uptime),
		"upvote":         commandFunc(upvote),
		"uq":             commandFunc(upquote),
		"userage":        commandFunc(userage),
		"ustime":         commandFunc(realTime),
		"voicekick":      commandFunc(voicekick),
		"votes":          commandFunc(votes),
		"weather":        commandFunc(weather),
		"whois":          commandFunc(whois),
		"willgrad":       commandFunc(willGrad),
		"xd":             commandFunc(xd),
		"zalgo":          commandFunc(ooer),
		string([]byte{119, 97, 116, 99, 104, 108, 105, 115, 116}): commandFunc(wlist),
	}
	newCommands := map[string]bool{
		"birdtime":       true,
		"fortune":        true,
		"karma":          true,
		"lastplayed":     true,
		"lastseen":       true,
		"math":           true,
		"mirotime":       true,
		"natime":         true,
		"nieltime":       true,
		"playtime":       true,
		"realtime":       true,
		"recentplaytime": true,
		"roll":           true,
		"sebbitime":      true,
		"top":            true,
		"topcommand":     true,
		"toplength":      true,
		"track":          true,
		"twintime":       true,
		"ustime":         true,
		"votes":          true,
		"weather":        true,
		"whois":          true,
	}

	executeCommand := func(s *discordgo.Session, guildID string, m *discordgo.MessageCreate, command []string) bool {
		commandName := strings.ToLower(command[0])
		if cmd, valid := funcMap[commandName]; valid {
			if guildID == "184428741450006528" || guildID == "161010139309015040" {
				if _, new := newCommands[commandName]; new {
					return true
				}
			}
			switch commandName {
			case "upvote", "downvote", "help", "commands", "command", "rename", "delete", "asuh", "uq", "uqquote", "bet", "permission", "voicekick", "timeout", "ignore", "mute", "unignore", "unmute", "pee":
			default:
				s.ChannelTyping(m.ChannelID)
			}
			reply, err := cmd(s, guildID, m.ChannelID, m.Author.ID, m.ID, command[1:])
			if err != nil {
				var errorID uuid.UUID
				if sqlErr := sqlClient.QueryRow(`INSERT INTO error(command, args, error) VALUES ($1, $2, $3) RETURNING id`, commandName, strings.Join(command[1:], " "), err.Error()).Scan(&errorID); sqlErr != nil {
					fmt.Println("ERROR recording error " + sqlErr.Error())
				}
				message, msgErr := s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("⚠ `%s`", err.Error()))
				if msgErr != nil {
					fmt.Println("ERROR SENDING ERROR MSG " + err.Error())
				} else {
					lastCommandMessages[m.Author.ID] = *m.Message
					lastMessagesByAuthor[m.Author.ID] = *message
				}
				fmt.Println("ERROR in " + commandName)
				fmt.Printf("ARGS: %v\n", command[1:])
				fmt.Println("ERROR: " + err.Error())
				return true
			}
			if len(reply) > 0 {
				message, err := s.ChannelMessageSend(m.ChannelID, reply)
				if err != nil {
					fmt.Println("ERROR sending message: " + err.Error())
					time.Sleep(500 * time.Millisecond)
					message, err = s.ChannelMessageSend(m.ChannelID, reply)
					if err != nil {
						fmt.Println("ERROR sending again ", err.Error())
						message, err = s.ChannelMessageSend(m.ChannelID, "⚠ `"+err.Error()+"`")
						if err != nil {
							fmt.Println("ERROR sending error")
						}
					}
				}
				lastCommandMessages[m.Author.ID] = *m.Message
				lastMessagesByAuthor[m.Author.ID] = *message
			}
			return true
		}
		return false
	}

	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		defer func() {
			if r := recover(); r != nil {
				fmt.Println(string(debug.Stack()))
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("⚠ `panic: %+v`⚠", r))
			}
		}()

		now := time.Now()
		fmt.Printf("%20s %20s %32s > %s\n", m.ChannelID, now.Format(time.Stamp), m.Author.Username, m.Content)

		messageID, err := strconv.ParseUint(m.ID, 10, 64)
		if err != nil {
			fmt.Println("ERROR parsing message ID " + err.Error())
			return
		}
		chanID, err := strconv.ParseUint(m.ChannelID, 10, 64)
		if err != nil {
			fmt.Println("ERROR parsing channel ID " + err.Error())
			return
		}
		authorID, err := strconv.ParseUint(m.Author.ID, 10, 64)
		if err != nil {
			fmt.Println("ERROR parsing author ID " + err.Error())
			return
		}
		if _, err = sqlClient.Exec(`INSERT INTO message (id, chan_id, author_id, content) VALUES ($1, $2, $3, $4)`,
			messageID, chanID, authorID, m.Content); err != nil {
			fmt.Println("ERROR inserting into Message")
			fmt.Println(err.Error())
		}

		if m.Author.ID == ownUserID {
			return
		}

		channel, err := s.State.Channel(m.ChannelID)
		if err != nil {
			if channel, err = s.Channel(m.ChannelID); err != nil {
				s.ChannelMessageSend(m.ChannelID, "⚠ `"+err.Error()+"`")
				fmt.Println("ERROR: " + err.Error())
				return
			}
		}
		if mutedUntil, found := mutedUserIDs[[2]string{channel.GuildID, m.Author.ID}]; found && mutedUntil.After(time.Now()) {
			s.ChannelMessageDelete(m.ChannelID, m.ID)
			return
		}

		/*if lastChanMessages, found := lastMessagesByChannel[m.ChannelID]; found {
			for _, msg := range lastChanMessages {
				if strings.ToLower(msg) == strings.ToLower(m.Content) {
					s.ChannelMessageDelete(m.ChannelID, m.ID)
					return
				}
			}
		}*/

		lastChanMessages := lastMessagesByChannel[m.ChannelID]
		for i := 1; i < len(lastChanMessages); i++ {
			lastChanMessages[i] = lastChanMessages[i-1]
		}
		lastChanMessages[0] = m.Message.Content
		lastMessagesByChannel[m.ChannelID] = lastChanMessages

		if ignoredUntil, found := ignoredUserIDs[[2]string{channel.GuildID, m.Author.ID}]; found && ignoredUntil.After(time.Now()) {
			return
		}

		/*if strings.Contains(strings.ToLower(m.Content), "texas") {
			s.ChannelMessageSend(m.ChannelID, ":gun: WEEHAW! :cowboy:")
		}*/

		/*if typingTimer, valid := typingTimer[m.Author.ID]; valid {
			typingTimer.Stop()
		}*/

		/*if strings.Contains(strings.ToLower(m.Content), "vape") || strings.Contains(strings.ToLower(m.Content), "v/\\") || strings.Contains(strings.ToLower(m.Content), "\\//\\") || strings.Contains(strings.ToLower(m.Content), "\\\\//\\") {
			s.ChannelMessageSend(m.ChannelID, "🆅🅰🅿🅴 🅽🅰🆃🅸🅾🅽")
		}*/
		if match := meanRegex.FindString(m.Content); match != "" {
			respond := rand.Intn(3)
			if respond == 0 {
				responses := []string{":(", "ayy fuck you too", "asshole.", "<@" + m.Author.ID + "> --"}
				_, err := s.ChannelMessageSend(m.ChannelID, responses[rand.Intn(len(responses))])
				if err != nil {
					fmt.Println("Error sending response " + err.Error())
				}
			}
		}
		if blocked, found := guildReactBlacklist[channel.GuildID]; !found || !blocked {
			if match := botRegex.FindString(m.Content); match != "" {
				if rand.Intn(8) == 0 {
					go func() {
						s.MessageReactionAdd(m.ChannelID, m.ID, "🤖")
						s.MessageReactionAdd(m.ChannelID, m.ID, "👋")
					}()
				}
			}
		}

		if match := commandRegex.FindStringSubmatch(m.Content); match != nil {
			if executeCommand(s, channel.GuildID, m, strings.Fields(match[1])) {
				return
			}
		}
		if match := questionRegex.FindString(m.Content); match != "" {
			executeCommand(s, channel.GuildID, m, []string{"8ball"})
			return
		}
		if match := inTheChatRegex.FindStringSubmatch(m.Content); match != nil {
			s.ChannelMessageSend(m.ChannelID, match[1])
			return
		}
		if match := upvoteRegex.FindStringSubmatch(m.Content); match != nil {
			executeCommand(s, channel.GuildID, m, []string{"upvote", match[1]})
			return
		}
		if match := downvoteRegex.FindStringSubmatch(m.Content); match != nil {
			executeCommand(s, channel.GuildID, m, []string{"downvote", match[1]})
			return
		}
		if match := kappaRegex.FindStringSubmatch(m.Content); match != nil {
			kappa(s, m.ChannelID, m.Author.ID, m.ID)
			return
		}
		if channel.Type == discordgo.ChannelTypeDM {
			executeCommand(s, channel.GuildID, m, strings.Fields(m.Content))
			return
		}
	}
}

func initGameUpdater(s *discordgo.Session) {
	res, err := http.Get("http://api.steampowered.com/ISteamApps/GetAppList/v2")
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	if res.StatusCode != 200 {
		fmt.Println(res.Status)
		return
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	res.Body.Close()
	var applist struct {
		Applist struct {
			Apps []struct {
				Appid int    `json:"appid"`
				Name  string `json:"name"`
			} `json:"apps"`
		} `json:"applist"`
	}
	err = json.Unmarshal(body, &applist)
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	gamelist = make([]string, len(applist.Applist.Apps))
	for i, app := range applist.Applist.Apps {
		gamelist[i] = app.Name
	}

	time.AfterFunc(time.Duration(960+rand.Intn(600))*time.Second, func() { updateGame(s) })
}

func updateGame(s *discordgo.Session) {
	defer time.AfterFunc(time.Duration(960+rand.Intn(600))*time.Second, func() { updateGame(s) })
	if currentGame != "" {
		changeGame := rand.Intn(3)
		if changeGame != 0 {
			return
		}
		currentGame = ""
	} else {
		index := rand.Intn(len(gamelist) * 5)
		if index >= len(gamelist) {
			currentGame = ""
		} else {
			currentGame = strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(gamelist[index]), "®", ""), "™", "")
		}
	}
	if err := s.UpdateGameStatus(0, currentGame); err != nil {
		fmt.Println("ERROR updating game: ", err.Error())
	}
}

func handleVoiceUpdate(s *discordgo.Session, v *discordgo.VoiceStateUpdate) {
	if _, err := sqlClient.Exec(`INSERT INTO voice_state (guild_id, chan_id, user_id, session_id, deaf, mute, self_deaf, self_mute, suppress) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		v.GuildID, v.ChannelID, v.UserID, v.SessionID, v.Deaf, v.Mute, v.SelfDeaf, v.SelfMute, v.Suppress); err != nil {
		fmt.Println("ERROR insert into VoiceState: ", err.Error())
	}
	if timeoutTime, found := timeoutedUserIDs[v.UserID]; found {
		if v.ChannelID != timeoutChanID && v.GuildID == timeoutGuildID && timeoutTime.Add(30*time.Second).After(time.Now()) {
			timeout := timeoutChanID
			s.GuildMemberMove(timeoutGuildID, v.UserID, &timeout)
		}
	}
}

func handleGuildMemberAdd(s *discordgo.Session, m *discordgo.GuildMemberAdd) {
	guild, err := s.Guild(m.GuildID)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	for _, channel := range guild.Channels {
		_, err := s.ChannelMessageSend(channel.ID, fmt.Sprintf("*%s has joined*", m.User.Username))
		if err == nil {
			break
		}
	}
}

func handleGuildMemberRemove(s *discordgo.Session, m *discordgo.GuildMemberRemove) {
	guild, err := s.Guild(m.GuildID)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	for _, channel := range guild.Channels {
		_, err := s.ChannelMessageSend(channel.ID, fmt.Sprintf("*%s has left*", m.User.Username))
		if err == nil {
			break
		}
	}
}

func handleGuildMemberUpdate(s *discordgo.Session, m *discordgo.GuildMemberUpdate) {
	if m.User.ID == ownUserID {
		fmt.Println("fixing self")
		if justNicknamed, found := wasNicknamed[m.GuildID]; found && justNicknamed {
			wasNicknamed[m.GuildID] = false
			return
		}
		var lastUsername string
		if err := sqlClient.QueryRow(`SELECT username FROM own_username WHERE guild_id = $1 ORDER BY create_date DESC LIMIT 1`, m.GuildID).Scan(&lastUsername); err != nil {
			if err == sql.ErrNoRows {
				lastUsername = "disgo"
			} else {
				fmt.Println("ERROR reverting update: getting old name", err)
				return
			}
		}
		if lastUsername == m.Nick {
			return
		}
		if err := s.GuildMemberNickname(m.GuildID, "@me/nick", lastUsername); err != nil {
			fmt.Println("ERROR reverting update: changing nick", err)
		}
	}
}

func handleMessageDelete(s *discordgo.Session, m *discordgo.MessageDelete) {
	messageID, err := strconv.ParseUint(m.ID, 10, 64)
	if err != nil {
		fmt.Println("ERROR parsing message ID " + err.Error())
		return
	}
	if _, err = sqlClient.Exec(`DELETE FROM message WHERE id = $1`, messageID); err != nil {
		fmt.Println("ERROR handling MessageDelete", err.Error())
		return
	}
}

func handleMessageUpdate(s *discordgo.Session, m *discordgo.MessageUpdate) {
	messageID, err := strconv.ParseUint(m.ID, 10, 64)
	if err != nil {
		fmt.Println("ERROR parsing message ID " + err.Error())
		return
	}
	if _, err = sqlClient.Exec(`UPDATE message SET content = $1 WHERE id = $2`, m.Content, messageID); err != nil {
		fmt.Println("ERROR handling MessageUpdate", err.Error())
		return
	}
}

func giveAllowance() {
	now := time.Now()
	nextRun := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.Local)
	time.AfterFunc(nextRun.Sub(now), giveAllowance)
	rows, err := sqlClient.Query(`SELECT guild_id, user_id FROM user_money`)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	defer rows.Close()
	var karmas []int
	var guildIDs, userIDs []string
	for rows.Next() {
		var guildID, userID string
		err := rows.Scan(&guildID, &userID)
		if err != nil {
			fmt.Println(err.Error())
			return
		}
		var karma int
		if err := sqlClient.QueryRow(`SELECT karma FROM user_karma WHERE guild_id = $1 AND user_id = $2`, guildID, userID).Scan(&karma); err != nil {
			if err == sql.ErrNoRows {
				karma = 0
			} else {
				fmt.Println(err.Error())
				return
			}
		}
		if karma < 0 {
			karma = 0
		}
		karmas = append(karmas, karma)
		guildIDs = append(guildIDs, guildID)
		userIDs = append(userIDs, userID)
	}
	for i := range karmas {
		if _, err = sqlClient.Exec(`UPDATE user_money SET money = money + $1 WHERE guild_id = $2 AND user_id = $3`, math.Max(3, 3+0.2*float64(karmas[i])), guildIDs[i], userIDs[i]); err != nil {
			fmt.Println(err.Error())
			return
		}
	}
}

func checkShipments(s *discordgo.Session) {
	defer time.AfterFunc(5*time.Minute, func() { checkShipments(s) })
	rows, err := sqlClient.Query(`SELECT id, carrier, tracking_number, chan_id, author_id FROM shipment`)
	if err != nil {
		fmt.Println("ERROR selecting from shipment", err)
	}
	defer rows.Close()
	var toDelete []int
	for rows.Next() {
		var ID int
		var carrier, trackingNum, chanID, authorID string
		if err := rows.Scan(&ID, &carrier, &trackingNum, &chanID, &authorID); err != nil {
			fmt.Println("ERROR scanning shipment", err)
			continue
		}
		status, err := getShippoTrack(carrier, trackingNum)
		if err != nil {
			fmt.Println("ERROR getting shipment status", err)
			continue
		}
		if status.TrackingStatus.Status == "DELIVERED" || status.TrackingStatus.Status == "FAILURE" {
			var statusStr string
			switch status.TrackingStatus.Status {
			case "DELIVERED":
				statusStr = "delivered"
			case "FAILURE":
				statusStr = "failed"
			}
			if _, err := s.ChannelMessageSend(chanID, fmt.Sprintf("<@%s>: Your %s shipment %s was marked as %s at %s with the following message: %s", authorID, status.Carrier, status.TrackingNumber, statusStr, status.TrackingStatus.StatusDate.Format(time.RFC1123Z), status.TrackingStatus.StatusDetails)); err != nil {
				fmt.Println("ERROR sending shipment message", err)
				continue
			}
			toDelete = append(toDelete, ID)
		}
	}
	if err := rows.Err(); err != nil {
		fmt.Println("ERROR calling next on rows", err)
	}
	for _, ID := range toDelete {
		if _, err := sqlClient.Exec(`DELETE FROM shipment WHERE id = $1`, ID); err != nil {
			fmt.Println("ERROR removing shipment", err)
			continue
		}
	}
}

func alexaAsuh(session *discordgo.Session) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		if string(body) != alexaBody {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		go asuh(session, "98470233999675392", "98470233999675392", "98482369446543360", "-1", []string{})
	}
}

func reportError(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query()["id"]
	if len(id) != 1 {
		fmt.Println("id doesn't have 1 value")
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	errorID, err := uuid.FromString(id[0])
	if err != nil {
		fmt.Println(err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	xForwardedFor := r.Header["X-Forwarded-For"]
	if len(xForwardedFor) != 1 {
		fmt.Println("X-Forward-For doesn't have 1 value")
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	sourceIP := xForwardedFor[0]
	if _, err := sqlClient.Exec(`INSERT INTO error_ip(error_id, ip) VALUES ($1, $2)`, errorID, sourceIP); err != nil {
		fmt.Println(err)
		if _, err := w.Write([]byte("Thank you!")); err != nil {
			fmt.Println(err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
		return
	}
	if _, err := sqlClient.Exec(`UPDATE error SET reported_count = reported_count + 1 WHERE id = $1`, errorID); err != nil {
		fmt.Println(err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	if _, err := w.Write([]byte("Thank you!")); err != nil {
		fmt.Println(err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
}

func main() {
	var err error
	sqlClient, err = sql.Open("postgres", fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s", dbUser, dbPass, dbHost, dbPort, dbName, dbSslMode))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	rand.Seed(time.Now().UnixNano())

	client, err := discordgo.New(botToken)
	if err != nil {
		fmt.Println(err)
		return
	}
	client.Identify.Intents =
		discordgo.IntentsGuilds |
			discordgo.IntentsGuildMembers |
			discordgo.IntentsGuildVoiceStates |
			discordgo.IntentsGuildPresences |
			discordgo.IntentsGuildMessages |
			discordgo.IntentsGuildMessageReactions |
			discordgo.IntentsGuildMessageTyping |
			discordgo.IntentsDirectMessages |
			discordgo.IntentsDirectMessageReactions |
			discordgo.IntentsDirectMessageTyping
	client.StateEnabled = true

	self, err := client.User("@me")
	if err != nil {
		fmt.Println(err)
		return
	}
	ownUserID = self.ID
	ownUserIDint, err = strconv.ParseUint(self.ID, 10, 64)
	if err != nil {
		fmt.Println("Failed to parse own ID " + err.Error())
		return
	}

	client.AddHandler(makeMessageCreate())
	client.AddHandler(handleVoiceUpdate)
	client.AddHandler(handleGuildMemberAdd)
	client.AddHandler(handleGuildMemberRemove)
	client.AddHandler(handleGuildMemberUpdate)
	client.AddHandler(handleMessageDelete)
	client.AddHandler(handleMessageUpdate)
	client.Open()
	defer client.Close()
	defer client.Logout()
	defer func() {
		voiceMutex.Lock()
		defer voiceMutex.Unlock()
		for guildID, voiceSession := range currentVoiceSessions {
			if voiceSession != nil {
				stopPlayer(guildID)
				err := voiceSession.Disconnect()
				if err != nil {
					fmt.Println("ERROR leaving voice channel " + err.Error())
				}
			}
		}
	}()

	signals := make(chan os.Signal, 1)

	go func() {
		<-signals
		voiceMutex.Lock()
		defer voiceMutex.Unlock()
		for guildID, voiceSession := range currentVoiceSessions {
			if voiceSession != nil {
				stopPlayer(guildID)
				err := voiceSession.Disconnect()
				if err != nil {
					fmt.Println("ERROR leaving voice channel " + err.Error())
				}
			}
		}
		client.Logout()
		client.Close()
		os.Exit(0)
	}()
	signal.Notify(signals, os.Interrupt)

	go initGameUpdater(client)

	now := time.Now()
	rows, err := sqlClient.Query("SELECT id, chan_id, author_id, send_time, content FROM reminder WHERE sent_at IS NULL")
	if err != nil {
		fmt.Println("ERROR setting reminders", err)
	}
	for rows.Next() {
		var id int32
		var chanID, authorID, content string
		var reminderTime time.Time
		err := rows.Scan(&id, &chanID, &authorID, &reminderTime, &content)
		if err != nil {
			fmt.Println("ERROR setting reminder", err)
			continue
		}
		var after time.Duration
		if reminderTime.Before(now) {
			after = 0
		} else {
			after = reminderTime.Sub(now)
		}
		time.AfterFunc(after, func() {
			client.ChannelMessageSend(chanID, fmt.Sprintf("<@%s> %s", authorID, content))
			sqlClient.Exec(`UPDATE reminder SET sent_at = now() WHERE id = $1`, id)
		})
	}
	rows.Close()

	now = time.Now()
	nextAllowance := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.Local)
	time.AfterFunc(nextAllowance.Sub(now), giveAllowance)

	time.AfterFunc(5*time.Minute, func() { checkShipments(client) })

	http.HandleFunc("/alexa_asuh", alexaAsuh(client))
	http.HandleFunc("/disgo_error", reportError)
	go func() {
		fmt.Println(http.ListenAndServe(":8083", nil))
	}()

	select {}
}
