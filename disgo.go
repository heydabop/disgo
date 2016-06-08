package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/Craftserve/mcstatus"
	"github.com/bwmarrin/dgvoice"
	"github.com/bwmarrin/discordgo"
	"github.com/gyuho/goling/similar"
	"github.com/james4k/rcon"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"image"
	imageColor "image/color"
	"image/png"
	"io/ioutil"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Command func(*discordgo.Session, string, string, string, []string) (string, error)
type KarmaDto struct {
	UserID string
	Karma  float64
}

type TwitchStreamReply struct {
	Stream *struct {
		ID         int     `json:"_id"`
		AverageFps float64 `json:"average_fps"`
		Game       string  `json:"game"`
		Viewers    int     `json:"viewers"`
		Channel    struct {
			DisplayName string `json:"display_name"`
			Name        string `json:"name"`
			Status      string `json:"status"`
		} `json:"channel"`
		VideoHeight int `json:"video_height"`
	} `json:"stream"`
}

type UserMessageCount struct {
	AuthorID    string
	NumMessages int64
}
type UserMessageLength struct {
	AuthorID  string
	AvgLength float64
}

type WolframQueryResult struct {
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

type UserMessageLengths []UserMessageLength

func (u UserMessageLengths) Len() int {
	return len(u)
}
func (u UserMessageLengths) Less(i, j int) bool {
	return u[i].AvgLength-u[j].AvgLength > 0
}
func (u UserMessageLengths) Swap(i, j int) {
	u[i], u[j] = u[j], u[i]
}

type SteamAppList struct {
	Applist struct {
		Apps []struct {
			Appid int    `json:"appid"`
			Name  string `json:"name"`
		} `json:"apps"`
	} `json:"applist"`
}

type RandomRandom struct {
	Data []int `json:"data"`
}
type RandomResult struct {
	Random RandomRandom `json:"random"`
}
type RandomResponse struct {
	Result RandomResult `json:"result"`
	ID     int          `json:"id"`
}

type UserBet struct {
	UserID         string
	WinningNumbers []int
	Payout         float64
	Bet            float64
}

var (
	currentGame                     string
	currentVoiceSession             *discordgo.VoiceConnection
	currentVoiceTimer               *time.Timer
	gamelist                        []string
	lastAuthorID                    string
	lastMessage, lastCommandMessage discordgo.Message
	lastQuoteIDs                    = make(map[string]int64)
	ownUserID                       string
	Rand                            = rand.New(rand.NewSource(time.Now().UnixNano()))
	rouletteGuildID                 = ""
	rouletteIsRed                   = []bool{true, false, true, false, true, false, true, false, true, false, false, true, false, true, false, true, false, true, true, false, true, false, true, false, true, false, true, false, false, true, false, true, false, true, false, true}
	rouletteBets                    []UserBet
	rouletteTableValues             = [][]int{{1, 2, 3}, {4, 5, 6}, {7, 8, 9}, {10, 11, 12}, {13, 14, 15}, {16, 17, 18}, {19, 20, 21}, {22, 23, 24}, {25, 26, 27}, {28, 29, 30}, {31, 32, 33}, {34, 35, 36}}
	rouletteWheelSpinning           = false
	rouletteWheelValues             = []int{32, 15, 19, 4, 12, 2, 25, 17, 34, 6, 27, 13, 36, 11, 30, 8, 23, 10, 5, 24, 16, 33, 1, 20, 14, 31, 9, 22, 18, 29, 7, 28, 12, 35, 3, 26, 0}
	startTime                       = time.Now()
	sqlClient                       *sql.DB
	typingTimer                     = make(map[string]*time.Timer)
	userIDRegex                     = regexp.MustCompile(`<@(\d+?)>`)
	userIDUpQuotes                  = make(map[string][]string)
	voiceMutex                      sync.Mutex
	voteTime                        = make(map[string]time.Time)
)

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

func getMostSimilarUserID(session *discordgo.Session, chanID, username string) (string, error) {
	channel, err := session.State.Channel(chanID)
	if err != nil {
		return "", err
	}
	guild, err := session.State.Guild(channel.GuildID)
	if err != nil {
		return "", err
	}
	var similarUsers []discordgo.User
	lowerUsername := strings.ToLower(username)
	if guild.Members != nil {
		for _, member := range guild.Members {
			if user := member.User; user != nil {
				if strings.Contains(strings.ToLower(user.Username), lowerUsername) {
					similarUsers = append(similarUsers, *user)
				}
			}
		}
	}
	if len(similarUsers) == 1 {
		return similarUsers[0].ID, nil
	}
	maxSim := 0.0
	maxUserID := ""
	usernameBytes := []byte(lowerUsername)
	for _, user := range similarUsers {
		sim := similar.Cosine([]byte(strings.ToLower(user.Username)), usernameBytes)
		if sim > maxSim {
			maxSim = sim
			maxUserID = user.ID
		}
	}
	if maxUserID == "" {
		return "", errors.New("No user found")
	}
	return maxUserID, nil
}

func getGameTimesFromRows(rows *sql.Rows) (UserMessageLengths, time.Time, int, error) {
	userGame := make(map[string]string)
	userTime := make(map[string]time.Time)
	gameTime := make(map[string]float64)
	firstTime := time.Now()
	for rows.Next() {
		var userID, timestamp, game string
		err := rows.Scan(&userID, &timestamp, &game)
		if err != nil {
			return make(UserMessageLengths, 0), time.Now(), 0, err
		}
		currTime, err := time.Parse(time.RFC3339Nano, timestamp)
		if err != nil {
			return make(UserMessageLengths, 0), time.Now(), 0, err
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
	gameTimes := make(UserMessageLengths, 0)
	longestGameLength := 0
	for game, time := range gameTime {
		gameTimes = append(gameTimes, UserMessageLength{game, time})
		if len(game) > longestGameLength {
			longestGameLength = len(game)
		}
	}
	sort.Sort(&gameTimes)
	return gameTimes, firstTime, longestGameLength, nil
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
	var karma float64
	err = sqlClient.QueryRow("SELECT Karma FROM UserKarma WHERE GuildId = ? AND UserId = ?", guildID, authorID).Scan(&karma)
	if err != nil {
		return -1, []int{}, err
	}
	if karma < bet {
		return -1, []int{}, errors.New("Like you can afford that.")
	}
	return bet, spaces, nil
}

func spam(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	if len(args) < 1 {
		cmd := exec.Command("find", "-iname", "*_nolink")
		cmd.Dir = "/home/ross/markov/"
		out, err := cmd.Output()
		if err != nil {
			return "", err
		}
		files := strings.Fields(string(out))
		for i := range files {
			files[i] = strings.Replace(files[i], "./", "", 1)
			files[i] = strings.Replace(files[i], "_nolink", "", 1)
		}
		sort.Strings(files)
		return strings.Join(files, ", "), nil
	}
	cmd := exec.Command("/home/ross/markov/1-markov.out", "1")
	logs, err := os.Open("/home/ross/markov/" + args[0] + "_nolink")
	if err != nil {
		return "", err
	}
	cmd.Stdin = logs
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func soda(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	return spam(session, chanID, authorID, messageID, []string{"sodapoppin"})
}

func lirik(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	return spam(session, chanID, authorID, messageID, []string{"lirik"})
}

func forsen(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	return spam(session, chanID, authorID, messageID, []string{"forsenlol"})
}

func cwc(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	return spam(session, chanID, authorID, messageID, []string{"cwc2016"})
}

func vote(session *discordgo.Session, chanID, authorID, messageID string, args []string, inc float64, isBet bool) (string, error) {
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
	channel, err := session.State.Channel(chanID)
	if err != nil {
		return "", err
	}
	_, err = session.GuildMember(channel.GuildID, userID)
	if err != nil {
		return "", err
	}
	if authorID != ownUserID {
		lastVoteTime, validTime := voteTime[authorID]
		if validTime && time.Since(lastVoteTime).Minutes() < 5+5*Rand.Float64() {
			return "Slow down champ.", nil
		}
	}
	if authorID != ownUserID && authorID == userID && inc > 0 {
		_, err := vote(session, chanID, ownUserID, messageID, []string{"<@" + authorID + ">"}, -1, false)
		if err != nil {
			return "", err
		}
		voteTime[authorID] = time.Now()
		return "No.", nil
	}

	var lastVoterIDAgainstUser, lastVoteTimestamp string
	var lastVoteTime time.Time
	err = sqlClient.QueryRow("select VoterId, Timestamp from Vote where GuildId = ? and VoteeId = ? order by Timestamp desc limit 1", channel.GuildID, authorID).Scan(&lastVoterIDAgainstUser, &lastVoteTimestamp)
	if err != nil {
		if err == sql.ErrNoRows {
			lastVoterIDAgainstUser = ""
		} else {
			return "", err
		}
	} else {
		lastVoteTime, err = time.Parse(time.RFC3339Nano, lastVoteTimestamp)
		if err != nil {
			return "", err
		}
	}
	if authorID != ownUserID && lastVoterIDAgainstUser == userID && time.Since(lastVoteTime).Hours() < 12 {
		return "Really?...", nil
	}
	var lastVoteeIDFromAuthor string
	err = sqlClient.QueryRow("select VoteeId, Timestamp from Vote where GuildId = ? and VoterId = ? order by Timestamp desc limit 1", channel.GuildID, authorID).Scan(&lastVoteeIDFromAuthor, &lastVoteTimestamp)
	if err != nil {
		if err == sql.ErrNoRows {
			lastVoteeIDFromAuthor = ""
		} else {
			return "", err
		}
	} else {
		lastVoteTime, err = time.Parse(time.RFC3339Nano, lastVoteTimestamp)
		if err != nil {
			return "", err
		}
	}
	if authorID != ownUserID && lastVoteeIDFromAuthor == userID && time.Since(lastVoteTime).Hours() < 12 {
		return "Really?...", nil
	}

	var karma float64
	err = sqlClient.QueryRow("select Karma from UserKarma where GuildId = ? and UserId = ?", channel.GuildID, userID).Scan(&karma)
	if err != nil {
		if err == sql.ErrNoRows {
			karma = 0
			_, insertErr := sqlClient.Exec("insert into UserKarma(GuildId, UserId, Karma) values (?, ?, ?)", channel.GuildID, userID, karma)
			if insertErr != nil {
				return "", err
			}
		} else {
			return "", err
		}
	}

	karma += inc
	_, err = sqlClient.Exec("update UserKarma set Karma = ? where GuildId = ? and UserId = ?", karma, channel.GuildID, userID)
	if err != nil {
		return "", err
	}
	voteTime[authorID] = time.Now()

	messageIDUnit, err := strconv.ParseUint(messageID, 10, 64)
	if err != nil {
		return "", err
	}
	if !isBet {
		isUpvote := false
		if inc > 0 {
			isUpvote = true
		}
		_, err = sqlClient.Exec("insert into Vote(GuildId, MessageId, VoterID, VoteeID, Timestamp, IsUpvote) values (?, ?, ?, ?, ?, ?)",
			channel.GuildID, messageIDUnit, authorID, userID, time.Now().Format(time.RFC3339Nano), isUpvote)
		if err != nil {
			return "", err
		}
	}
	return "", nil
}

func upvote(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	return vote(session, chanID, authorID, messageID, args, 1, false)
}

func downvote(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	return vote(session, chanID, authorID, messageID, args, -1, false)
}

func votes(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
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
	channel, err := session.State.Channel(chanID)
	if err != nil {
		return "", err
	}
	rows, err := sqlClient.Query("select UserId, Karma from UserKarma where GuildId = ? order by Karma desc limit ?", channel.GuildID, limit)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var votes []KarmaDto
	for rows.Next() {
		var userID string
		var karma float64
		err := rows.Scan(&userID, &karma)
		if err != nil {
			return "", err
		}
		votes = append(votes, KarmaDto{userID, karma})
	}
	finalString := ""
	for _, vote := range votes {
		user, err := session.User(vote.UserID)
		if err != nil {
			return "", err
		}
		finalString += fmt.Sprintf("%s — %.1f\n", user.Username, vote.Karma)
	}
	return finalString, nil
}

func roll(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	var max int64
	if len(args) < 1 {
		max = 6
	} else {
		var err error
		max, err = strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return "", err
		}
		if max <= 0 {
			return "", errors.New("Max roll must be more than 0")
		}
	}
	return fmt.Sprintf("%d", Rand.Int63n(max)+1), nil
}

func uptime(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	output, err := exec.Command("uptime").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func twitch(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	if len(args) < 1 {
		return "", errors.New("No stream provided")
	}
	streamName := args[0]
	res, err := http.Get(fmt.Sprintf("https://api.twitch.tv/kraken/streams/%s", url.QueryEscape(streamName)))
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
	var reply TwitchStreamReply
	err = json.Unmarshal(body, &reply)
	if err != nil {
		return "", err
	}
	if reply.Stream == nil {
		return "[Offline]", nil
	}
	return fmt.Sprintf(`%s playing %s
%s
%d viewers; %dp @ %.f FPS`, reply.Stream.Channel.Name, reply.Stream.Game, reply.Stream.Channel.Status, reply.Stream.Viewers, reply.Stream.VideoHeight, math.Floor(reply.Stream.AverageFps+0.5)), nil
}

func top(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
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
	rows, err := sqlClient.Query(`select AuthorId, count(AuthorId) as NumMessages from Message where ChanId = ? and Content not like '/%' group by AuthorId order by count(AuthorId) desc limit ?`, chanID, limit)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var counts []UserMessageCount
	for rows.Next() {
		var authorID string
		var numMessages int64
		err := rows.Scan(&authorID, &numMessages)
		if err != nil {
			return "", err
		}
		counts = append(counts, UserMessageCount{authorID, numMessages})
	}
	finalString := ""
	for _, count := range counts {
		user, err := session.User(count.AuthorID)
		if err != nil {
			return "", err
		}
		finalString += fmt.Sprintf("%s — %d\n", user.Username, count.NumMessages)
	}
	return finalString, nil
}

func topLength(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
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
	rows, err := sqlClient.Query(`select AuthorId, Content from Message where ChanId = ? and Content not like '/%' and trim(Content) != ''`, chanID)
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
	avgLengths := make(UserMessageLengths, 0)
	for userID, numMessages := range messagesPerUser {
		avgLengths = append(avgLengths, UserMessageLength{userID, float64(wordsPerUser[userID]) / float64(numMessages)})
	}
	sort.Sort(&avgLengths)
	finalString := ""
	for i, length := range avgLengths {
		if i >= limit {
			break
		}
		user, err := session.User(length.AuthorID)
		if err != nil {
			return "", err
		}
		finalString += fmt.Sprintf("%s — %.2f\n", user.Username, length.AvgLength)
	}
	return finalString, nil
}

func rename(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	if len(args) < 1 {
		return "", errors.New("No new username provided")
	}
	newUsername := strings.Join(args[0:], " ")
	var timestamp string
	var lockedMinutes int
	var lastChangeTime time.Time
	now := time.Now()
	err := sqlClient.QueryRow("select Timestamp, LockedMinutes from OwnUsername order by Timestamp desc limit 1").Scan(&timestamp, &lockedMinutes)
	if err != nil {
		if err == sql.ErrNoRows {
			lockedMinutes = 0
		} else {
			return "", err
		}
	} else {
		lastChangeTime, err = time.Parse(time.RFC3339Nano, timestamp)
		if err != nil {
			lastChangeTime = time.Now()
		}
	}

	if lockedMinutes == 0 || now.After(lastChangeTime.Add(time.Duration(lockedMinutes)*time.Minute)) {
		self, err := session.User("@me")
		if err != nil {
			return "", err
		}
		newSelf, err := session.UserUpdate(loginEmail, loginPassword, newUsername, self.Avatar, "")
		if err != nil {
			return "", err
		}

		channel, err := session.State.Channel(chanID)
		if err != nil {
			return "", err
		}
		var authorKarma float64
		err = sqlClient.QueryRow("select Karma from UserKarma where GuildId = ? and UserId = ?", channel.GuildID, authorID).Scan(&authorKarma)
		if err != nil {
			authorKarma = 0
		}
		newLockedMinutes := float64(Rand.Intn(30)) + 45 + 10*authorKarma
		if newLockedMinutes < 30 {
			newLockedMinutes = 30
		}

		_, err = sqlClient.Exec("INSERT INTO ownUsername (AuthorId, Timestamp, Username, LockedMinutes) values (?, ?, ?, ?)",
			authorID, now.Format(time.RFC3339Nano), newSelf.Username, newLockedMinutes)
		if err != nil {
			return "", err
		}
		author, err := session.User(authorID)
		if err != nil {
			return "", err
		}
		if authorKarma > 0 {
			return fmt.Sprintf("%s's name change will last for an extra %.f minutes thanks to their karma!", author.Username, 10*authorKarma), nil
		} else if authorKarma < 0 {
			return fmt.Sprintf("%s's name change will last up to %.f minutes less due to their karma...", author.Username, -10*authorKarma), nil
		}
	} else {
		return "I'm not ready to change who I am.", nil
	}
	return "", nil
}

func lastseen(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
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
	user, err := session.User(userID)
	if err != nil {
		return "", err
	}
	channel, err := session.State.Channel(chanID)
	if err != nil {
		return "", err
	}
	guild, err := session.State.Guild(channel.GuildID)
	if err != nil {
		return "", err
	}
	online := false
	for _, presence := range guild.Presences {
		if presence.User != nil && presence.User.ID == user.ID {
			online = presence.Status == "online"
			break
		}
	}
	if online {
		return fmt.Sprintf("%s is currently online", user.Username), nil
	}
	lastOnlineStr := ""
	err = sqlClient.QueryRow("select Timestamp from UserPresence where GuildId = ? and UserId = ? and (Presence = 'offline' or Presence = 'idle') order by Timestamp desc limit 1", guild.ID, userID).Scan(&lastOnlineStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Sprintf("%s was last seen at least %.f days ago", user.Username, time.Since(time.Date(2016, 4, 7, 1, 7, 0, 0, time.Local)).Hours()/24), nil
		}
		return "", err
	}
	lastOnline, err := time.Parse(time.RFC3339Nano, lastOnlineStr)
	if err != nil {
		return "", err
	}
	timeSince := time.Since(lastOnline)
	lastSeenStr := timeSinceStr(timeSince)
	return fmt.Sprintf("%s was last seen %s ago", user.Username, lastSeenStr), nil
}

func deleteLastMessage(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	if lastAuthorID == authorID {
		session.ChannelMessageDelete(lastMessage.ChannelID, lastMessage.ID)
		session.ChannelMessageDelete(lastCommandMessage.ChannelID, lastCommandMessage.ID)
		session.ChannelMessageDelete(chanID, messageID)
	}
	return "", nil
}

func kickme(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	channel, err := session.State.Channel(chanID)
	if err != nil {
		return "", err
	}
	err = session.GuildMemberDelete(channel.GuildID, authorID)
	if err != nil {
		return "", err
	}
	return "See ya nerd.", nil
}

func spamuser(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
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
	user, err := session.User(userID)
	if err != nil {
		return "", err
	}
	err = exec.Command("bash", "./gen_custom_log.sh", chanID, userID).Run()
	if err != nil {
		return "", err
	}
	cmd := exec.Command("/home/ross/markov/1-markov.out", "1")
	logs, err := os.Open("/home/ross/markov/" + userID + "_custom")
	if err != nil {
		return "", err
	}
	cmd.Stdin = logs
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	outStr := strings.TrimSpace(string(out))
	if match := regexp.MustCompile(`^(.*) ([[:punct:]])$`).FindStringSubmatch(outStr); match != nil {
		outStr = match[1] + match[2]
	}
	var numRows int64
	err = sqlClient.QueryRow(`select Count(Id) from Message where Content like ? and AuthorId = ?;`, "%"+outStr+"%", userID).Scan(&numRows)
	if err != nil {
		return "", err
	}
	freshStr := "stale meme :-1:"
	if numRows == 0 {
		freshStr = "💯％ CERTIFIED ＦＲＥＳＨ 👌"
	}
	res, err := sqlClient.Exec(`insert into DiscordQuote(ChanId, AuthorId, Content, Score, IsFresh) values (?, ?, ?, ?, ?)`, chanID, userID, outStr, 0, numRows == 0)
	if err != nil {
		fmt.Println("ERROR inserting into DiscordQuote ", err.Error())
	} else {
		quoteID, err := res.LastInsertId()
		if err != nil {
			fmt.Println("ERROR getting DiscordQuote ID ", err.Error())
		} else {
			lastQuoteIDs[chanID] = quoteID
			userIDUpQuotes[chanID] = make([]string, 0)
		}
	}
	return fmt.Sprintf("%s: %s\n%s", user.Username, freshStr, outStr), nil
}

func spamdiscord(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	err := exec.Command("bash", "./gen_custom_log_by_chan.sh", chanID).Run()
	if err != nil {
		return "", err
	}
	cmd := exec.Command("/home/ross/markov/1-markov.out", "1")
	logs, err := os.Open("/home/ross/markov/chan_" + chanID + "_custom")
	if err != nil {
		return "", err
	}
	cmd.Stdin = logs
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	outStr := strings.TrimSpace(string(out))
	if match := regexp.MustCompile(`^(.*) ([[:punct:]])$`).FindStringSubmatch(outStr); match != nil {
		outStr = match[1] + match[2]
	}
	var numRows int64
	err = sqlClient.QueryRow(`select Count(Id) from Message where Content like ? and ChanId = ? and AuthorId != ?;`, "%"+outStr+"%", chanID, ownUserID).Scan(&numRows)
	if err != nil {
		return "", err
	}
	freshStr := "stale meme :-1:"
	if numRows == 0 {
		freshStr = "💯％ CERTIFIED ＦＲＥＳＨ 👌"
	}
	res, err := sqlClient.Exec(`insert into DiscordQuote(ChanId, AuthorId, Content, Score, IsFresh) values (?, ?, ?, ?, ?)`, chanID, nil, outStr, 0, numRows == 0)
	if err != nil {
		fmt.Println("ERROR inserting into DiscordQuote ", err.Error())
	} else {
		quoteID, err := res.LastInsertId()
		if err != nil {
			fmt.Println("ERROR getting DiscordQuote ID ", err.Error())
		} else {
			lastQuoteIDs[chanID] = quoteID
			userIDUpQuotes[chanID] = make([]string, 0)
		}
	}
	return fmt.Sprintf("%s\n%s", freshStr, outStr), nil
}

func maths(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
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
	var response WolframQueryResult
	err = xml.Unmarshal(body, &response)
	if err != nil {
		return "", err
	}
	if response.NumPods == len(response.Pods) && response.NumPods > 0 {
		for _, pod := range response.Pods {
			if pod.Primary != nil && *(pod.Primary) == true {
				return pod.Plaintext, nil
			}
		}
	}
	return "", errors.New("No suitable answer found")
}

func cputemp(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	output, err := exec.Command("sensors", "coretemp-isa-0000").Output()
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(output), "\n")
	return fmt.Sprintf("```%s```", strings.Join(lines[2:], "\n")), nil
}

func ayy(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	return "lmao", nil
}

func ping(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	output, err := exec.Command("ping", "-qc3", "discordapp.com").Output()
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(output), "\n")
	return fmt.Sprintf("```%s```", strings.Join(lines[len(lines)-3:], "\n")), nil
}

func xd(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	return "PUCK FALMER", nil
}

func asuh(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	voiceMutex.Lock()
	defer voiceMutex.Unlock()

	channel, err := session.State.Channel(chanID)
	if err != nil {
		return "", err
	}
	guild, err := session.State.Guild(channel.GuildID)
	if err != nil {
		return "", err
	}
	voiceChanID := ""
	for _, state := range guild.VoiceStates {
		if state.UserID == authorID {
			voiceChanID = state.ChannelID
			break
		}
	}
	if voiceChanID == "" {
		return "I can't find which voice channel you're in.", nil
	}

	if currentVoiceSession != nil {
		if currentVoiceSession.ChannelID == voiceChanID && currentVoiceSession.GuildID == guild.ID {
			return "", nil
		}
		err = currentVoiceSession.Disconnect()
		currentVoiceSession = nil
		if err != nil {
			return "", err
		}
		time.Sleep(300 * time.Millisecond)
	}

	currentVoiceSession, err = session.ChannelVoiceJoin(guild.ID, voiceChanID, false, false)
	if err != nil {
		currentVoiceSession = nil
		return "", err
	}
	if currentVoiceTimer != nil {
		currentVoiceTimer.Stop()
	}
	currentVoiceTimer = time.AfterFunc(1*time.Minute, func() {
		if currentVoiceSession != nil {
			err := currentVoiceSession.Disconnect()
			currentVoiceSession = nil
			if err != nil {
				fmt.Println("ERROR disconnecting from voice channel " + err.Error())
			}
		}
	})

	time.Sleep(1 * time.Second)
	for i := 0; i < 10; i++ {
		if currentVoiceSession.Ready == false || currentVoiceSession.OpusSend == nil {
			time.Sleep(1 * time.Second)
			continue
		}
		suh := Rand.Intn(30)
		if err != nil {
			return "", err
		}
		dgvoice.PlayAudioFile(currentVoiceSession, fmt.Sprintf("suh%d.mp3", suh))
		break
	}
	session.ChannelMessageDelete(chanID, messageID)
	return "", nil
}

func upquote(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	lastQuoteID, found := lastQuoteIDs[chanID]
	if !found {
		return "I can't find what I spammed last.", nil
	}
	for _, userID := range userIDUpQuotes[chanID] {
		if userID == authorID {
			return "You've already upquoted my last spam", nil
		}
	}
	_, err := sqlClient.Exec(`update DiscordQuote set Score = Score + 1 WHERE Id = ?`, lastQuoteID)
	if err != nil {
		return "", err
	}
	userIDUpQuotes[chanID] = append(userIDUpQuotes[chanID], authorID)
	return "", nil
}

func topquote(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
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
	rows, err := sqlClient.Query(`select AuthorId, Content, Score from DiscordQuote where ChanId = ? and Score > 0 order by Score desc limit ?`, chanID, limit)
	if err != nil {
		return "", err
	}
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
			author, err := session.User(authorID.String)
			if err != nil {
				return "", err
			}
			authorName = author.Username
		}
		messages[i] = fmt.Sprintf("%s (%d): %s", authorName, score, content)
	}
	return strings.Join(messages[:i], "\n"), nil
}

func eightball(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	responses := []string{"It is certain", "It is decidedly so", "Without a doubt", "Yes, definitely", "You may rely on it", "As I see it, yes", "Most likely", "Outlook good", "Yes", "Signs point to yes", "Reply hazy try again", "Ask again later", "Better not tell you now", "Cannot predict now", "Concentrate and ask again", "Don't count on it", "My reply is no", "My sources say no", "Outlook not so good", "Very doubtful"}
	return responses[Rand.Intn(len(responses))], nil
}

func wlist(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
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
	rows, err := sqlClient.Query(`select AuthorId, Content from Message where ChanId = ? and Content not like '/%'`, chanID)
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
	var counts UserMessageLengths
	for authorID, score := range countMap {
		var numMessages int64
		err := sqlClient.QueryRow(`select count(Id) from Message where ChanId = ? and AuthorId = ? and Content not like '/%'`, chanID, authorID).Scan(&numMessages)
		if err != nil {
			return "", err
		}
		counts = append(counts, UserMessageLength{authorID, float64(score) / float64(numMessages)})
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
		author, err := session.User(counts[i].AuthorID)
		if err != nil {
			return "", err
		}
		output[i] = fmt.Sprintf("%s — %.4f", author.Username, counts[i].AvgLength)
	}
	return strings.Join(output, "\n"), nil
}

func oddshot(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	if len(args) < 1 {
		return "", errors.New("No oddshot url provided")
	}
	res, err := http.Get(fmt.Sprintf(args[0]))
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", errors.New(res.Status)
	}
	page, err := html.Parse(res.Body)
	if err != nil {
		return "", err
	}
	var provider, streamer, title, timestamp string
	var findTitle func(*html.Node)
	findTitle = func(n *html.Node) {
		if n.Type == html.ElementNode && len(n.Attr) > 0 {
			if p := n.FirstChild; p != nil && p.Type == html.TextNode {
				if n.DataAtom == atom.P && n.Attr[0].Key == "class" {
					if n.Attr[0].Val == "shot-title" {
						title = p.Data
					} else if n.Attr[0].Val == "shot-timestamp" {
						timestamp = p.Data
					}
				} else if n.DataAtom == atom.Span && n.Attr[0].Key == "id" {
					if n.Attr[0].Val == "providerID" {
						provider = p.Data
					} else if n.Attr[0].Val == "streamerID" {
						streamer = p.Data
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findTitle(c)
		}
	}
	findTitle(page)
	postedTime, err := time.Parse(time.RFC3339, timestamp)
	timeSince := timeSinceStr(time.Since(postedTime))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s: %s\n%s ago", provider, streamer, title, timeSince), nil
}

func remindme(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	arg := strings.Join(args, " ")
	fmt.Println(arg)
	atTimeRegex := regexp.MustCompile(`(?i)(?:at\s+)?(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}\s+[\+-]\d{4})\s+(?:to\s+)?(.*)`)
	inTimeRegex := regexp.MustCompile(`(?i)(?:in)?\s*(?:(?:(?:(\d+)\s+years?)|(?:(\d+)\s+months?)|(?:(\d+)\s+weeks?)|(?:(\d+)\s+days?)|(?:(\d+)\s+hours?)|(?:(\d+)\s+minutes?)|(?:(\d+)\s+seconds?))\s?)+(?:to)?\s+(.*)`)
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
		years, err = strconv.Atoi(inMatch[1])
		if err != nil {
			days = 0
		}
		months, err = strconv.Atoi(inMatch[2])
		if err != nil {
			days = 0
		}
		weeks, err = strconv.Atoi(inMatch[3])
		if err != nil {
			days = 0
		}
		days, err = strconv.Atoi(inMatch[4])
		if err != nil {
			days = 0
		}
		hours, err = strconv.ParseInt(inMatch[5], 10, 64)
		if err != nil {
			hours = 0
		}
		minutes, err = strconv.ParseInt(inMatch[6], 10, 64)
		if err != nil {
			minutes = 0
		}
		seconds, err = strconv.ParseInt(inMatch[7], 10, 64)
		if err != nil {
			seconds = 0
		}
		fmt.Printf("%dy %dm %dw %dd %dh %dm %ds\n", years, months, weeks, days, hours, minutes, seconds)
		remindTime = now.AddDate(years, months, weeks*7+days).Add(time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second)
	}
	fmt.Println(remindTime.Format(time.RFC3339))
	if remindTime.Before(now) {
		responses := []string{"Sorry, I lost my Delorean.", "Hold on, gotta hit 88MPH first.", "Too late.", "I'm sorry Dave, I can't do that.", ":|", "Time is a one-way street you idiot."}
		return responses[Rand.Intn(len(responses))], nil
	}
	_, err = sqlClient.Exec("INSERT INTO Reminder (ChanId, AuthorId, Time, Content) values (?, ?, ?, ?)", chanID, authorID, remindTime.In(time.FixedZone("UTC", 0)).Format(time.RFC3339), content)
	if err != nil {
		return "", err
	}
	time.AfterFunc(remindTime.Sub(now), func() { session.ChannelMessageSend(chanID, fmt.Sprintf("<@%s> %s", authorID, content)) })
	return fmt.Sprintf("👍 %s", remindTime.Format(time.RFC1123Z)), nil
}

func meme(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	var opID, link string
	err := sqlClient.QueryRow(`SELECT AuthorId, Content FROM Message WHERE ChanId = ? AND (Content LIKE 'http://%' OR Content LIKE 'https://%') AND AuthorId != ? ORDER BY RANDOM() LIMIT 1`, chanID, ownUserID).Scan(&opID, &link)
	if err != nil {
		return "", err
	}
	op, err := session.User(opID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s: %s", op.Username, link), nil
}

func bitrate(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	channel, err := session.State.Channel(chanID)
	if err != nil {
		return "", err
	}
	guildChans, err := session.GuildChannels(channel.GuildID)
	if err != nil {
		return "", err
	}
	var chanRates UserMessageLengths
	longestChanLength := 0
	for _, guildChan := range guildChans {
		if guildChan != nil && guildChan.Type == "voice" {
			chanRates = append(chanRates, UserMessageLength{guildChan.Name, float64(guildChan.Bitrate) / 1000})
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

func age(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
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
	channel, err := session.State.Channel(chanID)
	if err != nil {
		return "", err
	}
	member, err := session.GuildMember(channel.GuildID, userID)
	if err != nil {
		return "", err
	}
	if member.User == nil {
		return "", errors.New("No user found")
	}
	timeJoined, err := time.Parse(time.RFC3339Nano, member.JoinedAt)
	if err != nil {
		return "", err
	}
	timeSince := timeSinceStr(time.Now().Sub(timeJoined))
	return fmt.Sprintf("%s has been here for %s", member.User.Username, timeSince), nil
}

func lastUserMessage(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
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
	channel, err := session.State.Channel(chanID)
	if err != nil {
		return "", err
	}
	member, err := session.State.Member(channel.GuildID, userID)
	if err != nil {
		return "", err
	}
	if member.User == nil {
		return "", errors.New("No user found")
	}
	var timestamp string
	err = sqlClient.QueryRow("SELECT Timestamp FROM Message WHERE ChanId = ? AND AuthorId = ? ORDER BY Timestamp DESC LIMIT 1", chanID, userID).Scan(&timestamp)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Sprintf("I've never seen %s say anything.", member.User.Username), nil
		}
		return "", err
	}
	timeSent, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		return "", err
	}
	timeSince := timeSinceStr(time.Now().Sub(timeSent))
	return fmt.Sprintf("%s sent their last message %s ago", member.User.Username, timeSince), nil
}

func reminders(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	rows, err := sqlClient.Query("SELECT Time, Content FROM Reminder WHERE ChanId = ? AND AuthorId = ? AND Time > ? ORDER BY Time ASC", chanID, authorID, time.Now().In(time.FixedZone("UTC", 0)).Format(time.RFC3339Nano))
	if err != nil {
		return "", err
	}
	defer rows.Close()
	message := ""
	for rows.Next() {
		var content, timestamp string
		err = rows.Scan(&timestamp, &content)
		if err != nil {
			return "", err
		}
		remindTime, err := time.Parse(time.RFC3339Nano, timestamp)
		if err != nil {
			return "", err
		}
		message += fmt.Sprintf("%s — %s\n", remindTime.Format(time.RFC1123Z), content)
	}
	if len(message) < 1 {
		return "You have no pending reminders.", nil
	}
	privateChannel, err := session.UserChannelCreate(authorID)
	if err != nil {
		return "", err
	}
	_, err = session.ChannelMessageSend(privateChannel.ID, message)
	if err != nil {
		return "", err
	}
	session.ChannelMessageDelete(chanID, messageID)
	return "", nil
}

func color(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
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

func playtime(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	channel, err := session.State.Channel(chanID)
	if err != nil {
		return "", err
	}

	var limit int
	var userID string
	var rows *sql.Rows
	var user *discordgo.User
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
			user, err = session.User(userID)
			if err != nil {
				return "", err
			}
			rows, err = sqlClient.Query("SELECT UserId, Timestamp, Game FROM UserPresence WHERE GuildId = ? AND UserId = ? ORDER BY Timestamp ASC", channel.GuildID, userID)
		}
	}
	if rows == nil {
		rows, err = sqlClient.Query("SELECT UserId, Timestamp, Game FROM UserPresence WHERE GuildId = ? AND UserId != ? ORDER BY Timestamp ASC", channel.GuildID, ownUserID)
	}
	if err != nil {
		return "", err
	}
	defer rows.Close()

	gameTimes, firstTime, longestGameLength, err := getGameTimesFromRows(rows)
	if err != nil {
		return "", err
	}

	var message string
	if user != nil {
		message = fmt.Sprintf("%s since %s\n", user.Username, firstTime.Format(time.RFC1123Z))
	} else {
		message = fmt.Sprintf("Since %s\n", firstTime.Format(time.RFC1123Z))
	}
	for i := 0; i < limit && i < len(gameTimes); i++ {
		message += fmt.Sprintf("%"+strconv.Itoa(longestGameLength)+"s — %.2f\n", gameTimes[i].AuthorID, gameTimes[i].AvgLength)
	}
	return fmt.Sprintf("```%s```", message), nil
}

func recentPlaytime(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	now := time.Now()
	channel, err := session.State.Channel(chanID)
	if err != nil {
		return "", err
	}

	inTimeRegex := regexp.MustCompile(`(?i)(?:(?:(?:(\d+)\s+years?)|(?:(\d+)\s+months?)|(?:(\d+)\s+weeks?)|(?:(\d+)\s+days?)|(?:(\d+)\s+hours?)|(?:(\d+)\s+minutes?)|(?:(\d+)\s+seconds?))\s?)+(?:\s*(.*))?`)
	match := inTimeRegex.FindStringSubmatch(strings.Join(args, " "))
	if match == nil {
		return "What?", nil
	}
	fmt.Printf("%#v\n", match)
	selectionArg := match[8]
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

	limit := 10
	var userID string
	var rows *sql.Rows
	var user *discordgo.User
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
			user, err = session.User(userID)
			if err != nil {
				return "", err
			}
			rows, err = sqlClient.Query("SELECT UserId, Timestamp, Game FROM UserPresence WHERE GuildId = ? AND UserId = ? AND Timestamp > ? ORDER BY Timestamp ASC", channel.GuildID, userID, startTime.Format(time.RFC3339))
		}
	}
	if rows == nil {
		rows, err = sqlClient.Query("SELECT UserId, Timestamp, Game FROM UserPresence WHERE GuildId = ? AND UserId != ? AND Timestamp > ? ORDER BY Timestamp ASC", channel.GuildID, ownUserID, startTime.Format(time.RFC3339))
	}
	if err != nil {
		return "", err
	}
	defer rows.Close()

	gameTimes, _, longestGameLength, err := getGameTimesFromRows(rows)
	if err != nil {
		return "", err
	}

	var message string
	if user != nil {
		message = fmt.Sprintf("%s since %s\n", user.Username, startTime.Format(time.RFC1123Z))
	} else {
		message = fmt.Sprintf("Since %s\n", startTime.Format(time.RFC1123Z))
	}
	for i := 0; i < limit && i < len(gameTimes); i++ {
		message += fmt.Sprintf("%"+strconv.Itoa(longestGameLength)+"s — %.2f\n", gameTimes[i].AuthorID, gameTimes[i].AvgLength)
	}
	return fmt.Sprintf("```%s```", message), nil
}

func activity(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	var rows *sql.Rows
	var err error
	var username string
	if len(args) > 0 {
		var userID string
		var err error
		if match := userIDRegex.FindStringSubmatch(args[0]); match != nil {
			userID = match[1]
		} else {
			userID, err = getMostSimilarUserID(session, chanID, strings.Join(args, " "))
			if err != nil {
				return "", nil
			}
		}
		user, err := session.User(userID)
		if err != nil {
			return "", nil
		}
		username = user.Username
		rows, err = sqlClient.Query("SELECT Timestamp FROM MESSAGE WHERE ChanId = ? AND AuthorId = ? ORDER BY Timestamp asc", chanID, userID)
	} else {
		rows, err = sqlClient.Query("SELECT Timestamp FROM MESSAGE WHERE ChanId = ? AND AuthorId != ? ORDER BY Timestamp asc", chanID, ownUserID)
	}
	if err != nil {
		return "", err
	}
	defer rows.Close()
	channel, err := session.State.Channel(chanID)
	if err != nil {
		return "", err
	}
	hourCount := make([]uint64, 24, 24)
	var firstTime, msgTime time.Time
	if rows.Next() {
		var timestamp string
		err = rows.Scan(&timestamp)
		if err != nil {
			return "", err
		}
		firstTime, err = time.Parse(time.RFC3339Nano, timestamp)
		firstTime = firstTime.Local()
		if err != nil {
			return "", err
		}
		hourCount[firstTime.Hour()]++
	}
	for rows.Next() {
		var timestamp string
		err = rows.Scan(&timestamp)
		if err != nil {
			return "", err
		}
		msgTime, err = time.Parse(time.RFC3339Nano, timestamp)
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
	plotFile, err := ioutil.TempFile("", "disgo")
	if err != nil {
		return "", err
	}
	defer os.Remove(plotFile.Name())
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
		return "", nil
	}
	_, err = session.ChannelFileSend(chanID, plotFile.Name()+".png", plotFile)
	if err != nil {
		return "", err
	}
	return "", nil
}

func botuptime(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	uptime := time.Since(startTime)
	days := "days"
	if math.Floor(uptime.Hours()/24) == 1 {
		days = "day"
	}
	return fmt.Sprintf("%.f %s %02d:%02d", math.Floor(uptime.Hours()/24), days, uint64(math.Floor(uptime.Hours()))%24, uint64(math.Floor(uptime.Minutes()))%60), nil
}

func nest(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	dateStr := time.Now().Format("20060102")
	cmd := exec.Command("/home/ross/.gocode/src/github.com/heydabop/nesttracking/graph/graph", dateStr)
	cmd.Dir = "/home/ross/.gocode/src/github.com/heydabop/nesttracking/graph/"
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s.png", nestlogRoot, dateStr), nil
}

func minecraft(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	channel, err := session.State.Channel(chanID)
	if err != nil {
		return "", err
	}
	guild, err := session.State.Guild(channel.GuildID)
	if err != nil {
		return "", err
	}
	if guild.ID != minecraftGuildID {
		return "", nil
	}
	server, err := net.ResolveTCPAddr("tcp4", fmt.Sprintf("127.0.0.1:25565"))
	if err != nil {
		return "", err
	}
	status, _, err := mcstatus.CheckStatus(server)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%d\n[%d/%d] Online: %s", minecraftServer, minecraftPort, status.Players, status.Slots, strings.Join(status.PlayersSample, ", ")), nil
}

func roulette(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	if rouletteWheelSpinning {
		return "Wheel is already spinning, place a bet", nil
	}
	channel, err := session.State.Channel(chanID)
	if err != nil {
		return "", err
	}
	guild, err := session.State.Guild(channel.GuildID)
	if err != nil {
		return "", err
	}
	res, err := http.Post("https://api.random.org/json-rpc/1/invoke", "application/json", strings.NewReader(`{"jsonrpc": "2.0","method": "generateIntegers","params": {"apiKey": "9f397d6a-c4bd-49b6-9f9c-621183b2d2e1","n": 1,"min": 0,"max": 36},"id": `+messageID+`}`))
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", errors.New(res.Status)
	}
	body, err := ioutil.ReadAll(res.Body)
	var result RandomResponse
	err = json.Unmarshal(body, &result)
	if err != nil {
		return "", nil
	}
	if strconv.Itoa(result.ID) != messageID {
		return "", errors.New("ID mismatch")
	}
	value := rouletteWheelValues[result.Result.Random.Data[0]]
	colorStr := "Black"
	if value != 0 && rouletteIsRed[value-1] {
		colorStr = "Red"
	}
	time.AfterFunc(45*time.Second, func() {
		if value == 0 {
			session.ChannelMessageSend(chanID, "Landed on 0")
		} else {
			session.ChannelMessageSend(chanID, fmt.Sprintf("%s %d", colorStr, value))
		}
		winner := false
		for _, bet := range rouletteBets {
			betWin := false
			for _, betSpace := range bet.WinningNumbers {
				if betSpace == value {
					user, err := session.User(bet.UserID)
					if err == nil {
						winner, betWin = true, true
						session.ChannelMessageSend(chanID, fmt.Sprintf("%s wins %.1f more karma!", user.Username, bet.Payout*bet.Bet))
						_, err := vote(session, chanID, ownUserID, messageID, []string{"<@" + bet.UserID + ">"}, (bet.Payout+1)*bet.Bet, true)
						if err != nil {
							session.ChannelMessageSend(chanID, "⚠ `"+err.Error()+"`")
						}
						break
					}
				}
			}
			if !betWin {
				_, err := vote(session, chanID, ownUserID, messageID, []string{"<@" + ownUserID + ">"}, bet.Bet, true)
				if err != nil {
					session.ChannelMessageSend(chanID, "⚠ `"+err.Error()+"`")
				}
			}
		}
		if len(rouletteBets) > 0 && winner == false {
			session.ChannelMessageSend(chanID, "Everyone loses!")
		}
		rouletteBets = make([]UserBet, 0)
		rouletteWheelSpinning = false
	})
	rouletteGuildID = guild.ID
	rouletteWheelSpinning = true
	return "Spinning...", nil
}

func bet(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	channel, err := session.State.Channel(chanID)
	if err != nil {
		return "", err
	}
	guild, err := session.State.Guild(channel.GuildID)
	if err != nil {
		return "", err
	}
	if len(args) < 2 {
		privateChannel, err := session.UserChannelCreate(authorID)
		if err != nil {
			return "", err
		}
		_, err = session.ChannelMessageSend(privateChannel.ID, `The following bet types are allowed
All of these are proceeded with /bet <amount>, amount being how much karma you bet >= 0.1
Inside
`+"```"+`Straight - single <number> - payout if ball lands on given number, 0-36 inclusive - /bet 0.5 single 13
Split - split <number> <number> - on 1 of the 2 adjacent numbers - /bet 0.7 split 16 17
Street - street <number> - on 1 of the numbers in same row as given number - /bet 0.4 street 13
Corner - corner <number> <number> <number> <number> - on one of 4 given adjacent numbers - /bet 1 corner 25 26 28 29
Six Line - six <number> <number> - on one of 6 numbers from adjacent rows in which the 2 given numbers lie - /bet 1.5 six 13 16
Trio - trio <number> - on 0 or given number (1, 2, or 3) - /bet 1.2 trio 2
Basket - basket - on 0, 1, 2, or 3`+"```"+`
Outside *COMING SOON*
`+"```"+`Low - low - on 1-18
High - high - on 19-36
Red - red - on red
Black - black - on black
Even - even - on even
Odd - odd - on odd
Dozen - dozen <number> - on 1 of 12 in same dozen group (1-12, 13-24, 25-36) as given number - /bet 3 dozen 1
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
	if !rouletteWheelSpinning {
		return "The wheel must be spinning to place a bet. Try /spin", nil
	}
	if guild.ID != rouletteGuildID { //TODO: dont be lazy and allow multiple wheels
		return "", errors.New("Sorry, the wheel is spinning in another server...")
	}
	var bet float64
	var spaces []int
	switch strings.ToLower(args[1]) {
	case "single":
		bet, spaces, err = getBetDetails(guild.ID, authorID, append(args[:1], args[2:]...), 1)
		if err != nil {
			return "", err
		}
		rouletteBets = append(rouletteBets, UserBet{authorID, spaces, 35, bet})
	case "split":
		if len(args) < 3 {
			return "", errors.New("Missing number(s) in split bet")
		}
		bet, spaces, err = getBetDetails(guild.ID, authorID, append(args[:1], args[2:]...), 2)
		if err != nil {
			return "", err
		}
		if (spaces[0] != spaces[1]) && (((spaces[0]-1)/3 == (spaces[1]-1)/3 && int(math.Abs(float64(spaces[1]-spaces[0]))) == 1) || int(math.Abs(float64(spaces[1]-spaces[0]))) == 3 || ((spaces[0] == 0 || spaces[1] == 0) && int(math.Abs(float64(spaces[1]-spaces[0]))) <= 3)) {
			rouletteBets = append(rouletteBets, UserBet{authorID, spaces, 17, bet})
		} else {
			return "", fmt.Errorf("Spaces %v aren't adjacent", spaces)
		}
	case "street":
		bet, spaces, err = getBetDetails(guild.ID, authorID, append(args[:1], args[2:]...), 1)
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
		rouletteBets = append(rouletteBets, UserBet{authorID, spaces, 11, bet})
	case "corner":
		bet, spaces, err = getBetDetails(guild.ID, authorID, append(args[:1], args[2:]...), 4)
		if err != nil {
			return "", err
		}
		for _, space := range spaces {
			if space == 0 {
				return "", errors.New("Can't corner bet on 0")
			}
		}
		if spaces[1]-spaces[0] != 1 || spaces[3]-spaces[2] != 1 || (spaces[0]-1)/3 != (spaces[1]-1)/3 || (spaces[2]-1)/3 != (spaces[3]-1)/3 || ((spaces[2]-1)/3)-((spaces[0]-1)/3) != 1 || ((spaces[3]-1)/3)-((spaces[1]-1)/3) != 1 {
			return "", errors.New("Spaces %v aren't all adjacent. Note that spaces should be entered in ascending order. 16 17 19 20 isn't treated the same as 19 20 16 17")
		}
		rouletteBets = append(rouletteBets, UserBet{authorID, spaces, 8, bet})
	case "six":
		bet, spaces, err = getBetDetails(guild.ID, authorID, append(args[:1], args[2:]...), 2)
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
		rouletteBets = append(rouletteBets, UserBet{authorID, betSpaces, 5, bet})
	case "trio":
		bet, spaces, err = getBetDetails(guild.ID, authorID, append(args[:1], args[2:]...), 1)
		if err != nil {
			return "", err
		}
		if spaces[0] != 1 && spaces[0] != 2 && spaces[0] != 3 {
			return "", errors.New("Trio bet is only valid on 1, 2, or 3")
		}
		rouletteBets = append(rouletteBets, UserBet{authorID, spaces, 11, bet})
	case "basket":
		bet, _, err = getBetDetails(guild.ID, authorID, append(args[:1], args[2:]...), 0)
		if err != nil {
			return "", err
		}
		rouletteBets = append(rouletteBets, UserBet{authorID, []int{0, 1, 2, 3}, 11, bet})
	default:
		return "", errors.New("Unrecognized bet type")
	}
	_, err = vote(session, chanID, ownUserID, messageID, []string{"<@" + authorID + ">"}, -bet, true)
	if err != nil {
		return "", err
	}
	return "", nil
}

func topcommand(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	if len(args) < 1 {
		return "", errors.New("No command provided")
	}
	rows, err := sqlClient.Query(`SELECT AuthorId, COUNT(AuthorId) FROM Message WHERE Content LIKE ? AND ChanId = ? GROUP BY AuthorId ORDER BY COUNT(AuthorId) DESC`, fmt.Sprintf(`/%s%%`, args[0]), chanID)
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
		user, err := session.User(userID)
		if err != nil {
			return "", err
		}
		message += fmt.Sprintf("%s — %d\n", user.Username, count)
	}
	return message, nil
}

func help(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	privateChannel, err := session.UserChannelCreate(authorID)
	if err != nil {
		return "", err
	}
	_, err = session.ChannelMessageSend(privateChannel.ID, `**activity** - shows average messages per hour over lifetime of channel
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
**karma** [number (optional)] - displays top <number> users and their karma
**lastseen** [username] - displays when <username> was last seen
**lastmessage** [username] - displays when <username> last sent a message
**lirik** - alias for /spam lirik
**math** [math stuff] - does math
**meme** - random meme from channel history
**ping** - displays ping to discordapp.com
**playtime** [number (optional)] OR [username (options)] - shows up to <number> summated (probably incorrect) playtimes in hours of every game across all users, or top 10 games of <username>
**recentplaytime** [duration] [[number (optional)] OR [username (options)]] - same as playtime but with a duration (like remindme) before normal args, calculates only as far back as duration`)
	if err != nil {
		return "", err
	}
	_, err = session.ChannelMessageSend(privateChannel.ID, `**remindme**
	in [duration] to [x] - mentions user with <x> after <duration> (example: /remindme in 5 hours 10 minutes 3 seconds to order a pizza)
	at [time] to [x] - mentions user with <x> at <time> (example: /remindme at 2016-05-04 13:37:00 -0500 to make a clever xd facebook status)
**reminders** - messages you your pending reminders
**rename** [new username] - renames bot
**roll** [sides (optional)] - "rolls" a die with <sides> sides
**spam** [streamer (optional)] - generates a messages based on logs from <streamer>, shows all streamer logs if no streamer is specified
**spamdiscord** - generates a message based on logs from this discord channel
**spamuser** [username] - generates a message based on discord logs of <username>
**spin** or **roulette** - spin roulette wheel
**soda** - alias for /spam sodapoppin
**top** [number (optional)] - displays top <number> users sorted by messages sent
**topCommand** [command] - displays who has issued <command> most
**topLength** [number (optional)] - dispalys top <number> users sorted by average words/message
**topQuote** [number (optional)] - dispalys top <number> of "quotes" from bot spam, sorted by votes from /upquote
**twitch** [channel] - displays info about twitch channel
**uptime** - displays bot's server uptime and load
**upquote** - upvotes last statement generated by /spamuser or /spamdiscord
**uq** - alias for /upquote
**upvote** [@user] - upvotes user
**@[user]++** - upvotes user
**votes** [number (optional)] - displays top <number> users and their karma
`+string([]byte{42, 42, 119, 97, 116, 99, 104, 108, 105, 115, 116, 42, 42, 32, 91, 110, 117, 109, 98, 101, 114, 32, 40, 111, 112, 116, 105, 111, 110, 97, 108, 41, 93, 32, 45, 32, 100, 105, 115, 112, 108, 97, 121, 115, 32, 116, 111, 112, 32, 60, 110, 117, 109, 98, 101, 114, 62, 32, 117, 115, 101, 114, 115, 32, 115, 111, 114, 116, 101, 100, 32, 98, 121, 32, 116, 101, 114, 114, 111, 114, 105, 115, 109, 32, 112, 101, 114, 32, 109, 101, 115, 115, 97, 103, 101})+`
**xd**`)
	if err != nil {
		return "", err
	}
	session.ChannelMessageDelete(chanID, messageID)
	return "", nil
}

func makeMessageCreate() func(*discordgo.Session, *discordgo.MessageCreate) {
	regexes := []*regexp.Regexp{regexp.MustCompile(`^<@` + ownUserID + `>\s+(.+)`), regexp.MustCompile(`^\/(.+)`)}
	upvoteRegex := regexp.MustCompile(`(<@\d+?>)\s*\+\+`)
	downvoteRegex := regexp.MustCompile(`(<@\d+?>)\s*--`)
	twitchRegex := regexp.MustCompile(`(?i)https?:\/\/(www\.)?twitch.tv\/(\w+)`)
	//oddshotRegex := regexp.MustCompile(`(?i)https?:\/\/(www\.)?oddshot.tv\/shot\/[\w-]+`)
	meanRegex := regexp.MustCompile(`(?i)((fuc)|(shit)|(garbage)|(garbo)).*bot($|[[:space:]])`)
	questionRegex := regexp.MustCompile(`^<@` + ownUserID + `>.*\w+.*\?$`)
	inTheChatRegex := regexp.MustCompile(`(?i)can i get a\s+(.*?)\s+in the chat`)
	funcMap := map[string]Command{
		"spam":           Command(spam),
		"soda":           Command(soda),
		"lirik":          Command(lirik),
		"forsen":         Command(forsen),
		"roll":           Command(roll),
		"help":           Command(help),
		"upvote":         Command(upvote),
		"downvote":       Command(downvote),
		"votes":          Command(votes),
		"karma":          Command(votes),
		"uptime":         Command(uptime),
		"twitch":         Command(twitch),
		"top":            Command(top),
		"toplength":      Command(topLength),
		"rename":         Command(rename),
		"lastseen":       Command(lastseen),
		"delete":         Command(deleteLastMessage),
		"cwc":            Command(cwc),
		"kickme":         Command(kickme),
		"spamuser":       Command(spamuser),
		"math":           Command(maths),
		"cputemp":        Command(cputemp),
		"ayy":            Command(ayy),
		"spamdiscord":    Command(spamdiscord),
		"ping":           Command(ping),
		"xd":             Command(xd),
		"asuh":           Command(asuh),
		"upquote":        Command(upquote),
		"uq":             Command(upquote),
		"topquote":       Command(topquote),
		"8ball":          Command(eightball),
		"oddshot":        Command(oddshot),
		"remindme":       Command(remindme),
		"meme":           Command(meme),
		"bitrate":        Command(bitrate),
		"commands":       Command(help),
		"command":        Command(help),
		"age":            Command(age),
		"lastmessage":    Command(lastUserMessage),
		"reminders":      Command(reminders),
		"color":          Command(color),
		"playtime":       Command(playtime),
		"recentplaytime": Command(recentPlaytime),
		"activity":       Command(activity),
		"botuptime":      Command(botuptime),
		"nest":           Command(nest),
		"minecraft":      Command(minecraft),
		"roulette":       Command(roulette),
		"bet":            Command(bet),
		"spin":           Command(roulette),
		"topcommand":     Command(topcommand),
		string([]byte{119, 97, 116, 99, 104, 108, 105, 115, 116}): Command(wlist),
	}

	executeCommand := func(s *discordgo.Session, m *discordgo.MessageCreate, command []string) {
		if cmd, valid := funcMap[strings.ToLower(command[0])]; valid {
			switch command[0] {
			case "upvote", "downvote", "help", "commands", "command", "rename", "delete", "asuh", "uq", "uqquote", "reminders", "bet":
			default:
				s.ChannelTyping(m.ChannelID)
			}
			reply, err := cmd(s, m.ChannelID, m.Author.ID, m.ID, command[1:])
			if err != nil {
				message, msgErr := s.ChannelMessageSend(m.ChannelID, "⚠ `"+err.Error()+"`")
				if msgErr != nil {
					fmt.Println("ERROR SENDING ERROR MSG " + err.Error())
				} else {
					lastCommandMessage = *m.Message
					lastMessage = *message
					lastAuthorID = m.Author.ID
				}
				fmt.Println("ERROR in " + command[0])
				fmt.Printf("ARGS: %v\n", command[1:])
				fmt.Println("ERROR: " + err.Error())
				return
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
				lastCommandMessage = *m.Message
				lastMessage = *message
				lastAuthorID = m.Author.ID
			}
			return
		}
	}

	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		now := time.Now()
		fmt.Printf("%20s %20s %20s > %s\n", m.ChannelID, now.Format(time.Stamp), m.Author.Username, m.Content)

		messageID, err := strconv.ParseUint(m.ID, 10, 64)
		if err != nil {
			fmt.Println("ERROR parsing message ID " + err.Error())
			return
		}
		_, err = sqlClient.Exec("INSERT INTO Message (Id, ChanId, AuthorId, Timestamp, Content) values (?, ?, ?, ?, ?)",
			messageID, m.ChannelID, m.Author.ID, now.Format(time.RFC3339Nano), m.Content)
		if err != nil {
			fmt.Println("ERROR inserting into Message")
			fmt.Println(err.Error())
		}
		err = s.ChannelMessageAck(m.ChannelID, m.ID)
		if err != nil {
			fmt.Println("Error ACKing message", err.Error())
		}

		if m.Author.ID == ownUserID {
			return
		}

		/*if typingTimer, valid := typingTimer[m.Author.ID]; valid {
			typingTimer.Stop()
		}*/

		/*if strings.Contains(strings.ToLower(m.Content), "vape") || strings.Contains(strings.ToLower(m.Content), "v/\\") || strings.Contains(strings.ToLower(m.Content), "\\//\\") || strings.Contains(strings.ToLower(m.Content), "\\\\//\\") {
			s.ChannelMessageSend(m.ChannelID, "🆅🅰🅿🅴 🅽🅰🆃🅸🅾🅽")
		}*/
		if m.ChannelID == minecraftChanID {
			username, found := minecraftUsernameMap[m.Author.ID]
			if !found {
				if len(m.Author.Username) > 16 {
					username = fmt.Sprintf("%s…", m.Author.Username[:16])
				} else {
					username = m.Author.Username
				}
			}
			conn, err := rcon.Dial(fmt.Sprintf("127.0.0.1:%s", mcrconPort), mcrconPass)
			if err == nil {
				_, err := conn.Write(fmt.Sprintf("say %s> %s", username, m.Content))
				if err != nil {
					s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("`ERROR BRIDGING MESSAGE: %s", err.Error()))
					fmt.Println(err.Error())
				}
			} else {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("`ERROR BRIDGING MESSAGE: %s", err.Error()))
				fmt.Println(err.Error())
			}
		}
		if match := meanRegex.FindString(m.Content); match != "" {
			respond := Rand.Intn(3)
			if respond == 0 {
				responses := []string{":(", "ayy fuck you too", "asshole.", "<@" + m.Author.ID + "> --"}
				_, err := s.ChannelMessageSend(m.ChannelID, responses[Rand.Intn(len(responses))])
				if err != nil {
					fmt.Println("Error sending response " + err.Error())
				}
			}
		}

		if match := questionRegex.FindString(m.Content); match != "" {
			executeCommand(s, m, []string{"8ball"})
			return
		}
		if match := inTheChatRegex.FindStringSubmatch(m.Content); match != nil {
			s.ChannelMessageSend(m.ChannelID, match[1])
			return
		}
		if match := upvoteRegex.FindStringSubmatch(m.Content); match != nil {
			executeCommand(s, m, []string{"upvote", match[1]})
			return
		}
		if match := downvoteRegex.FindStringSubmatch(m.Content); match != nil {
			executeCommand(s, m, []string{"downvote", match[1]})
			return
		}
		if match := twitchRegex.FindStringSubmatch(m.Content); match != nil {
			executeCommand(s, m, []string{"twitch", match[2]})
			return
		}
		/*if match := oddshotRegex.FindString(m.Content); match != "" {
			executeCommand(s, m, []string{"oddshot", match})
			return
		}*/
		for _, regex := range regexes {
			if match := regex.FindStringSubmatch(m.Content); match != nil {
				executeCommand(s, m, strings.Fields(match[1]))
				return
			}
		}
	}
}

func initGameUpdater(s *discordgo.Session) {
	res, err := http.Get(fmt.Sprintf("http://api.steampowered.com/ISteamApps/GetAppList/v2"))
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
	var applist SteamAppList
	err = json.Unmarshal(body, &applist)
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	gamelist = make([]string, len(applist.Applist.Apps))
	for i, app := range applist.Applist.Apps {
		gamelist[i] = app.Name
	}

	go updateGame(s)
}

func updateGame(s *discordgo.Session) {
	if currentGame != "" {
		changeGame := Rand.Intn(3)
		if changeGame != 0 {
			time.AfterFunc(time.Duration(480+Rand.Intn(300))*time.Second, func() { updateGame(s) })
			return
		}
		currentGame = ""
	} else {
		index := Rand.Intn(len(gamelist) * 5)
		if index >= len(gamelist) {
			currentGame = ""
		} else {
			currentGame = gamelist[index]
		}
	}
	userGuilds, err := s.UserGuilds()
	if err != nil {
		fmt.Println("Error getting user guilds", err.Error())
	}

	now := time.Now()
	err = s.UpdateStatus(0, currentGame)
	if err != nil {
		fmt.Println("ERROR updating game: ", err.Error())
	}
	for _, guild := range userGuilds {
		_, err := sqlClient.Exec("INSERT INTO UserPresence (GuildId, UserId, Timestamp, Presence, Game) values (?, ?, ?, ?, ?)", guild.ID, ownUserID, now.Format(time.RFC3339Nano), "online", currentGame)
		if err != nil {
			fmt.Println("ERROR inserting self into UserPresence DB")
			fmt.Println(err.Error())
		}
	}
	time.AfterFunc(time.Duration(480+Rand.Intn(300))*time.Second, func() { updateGame(s) })
}

func handlePresenceUpdate(s *discordgo.Session, p *discordgo.PresenceUpdate) {
	now := time.Now()
	if p.User == nil {
		return
	}
	if p.User.ID == ownUserID { //doesnt happen now, might later, prevent double insertions
		return
	}
	gameName := ""
	if p.Game != nil {
		gameName = p.Game.Name
	}
	/*user, err := s.User(p.User.ID)
	if err != nil {
		fmt.Println("ERROR getting user")
		fmt.Println(err.Error())
	} else {
		fmt.Printf("%20s %20s %20s : %s %s\n", p.GuildID, now.Format(time.Stamp), user.Username, p.Status, gameName)
	}*/
	_, err := sqlClient.Exec("INSERT INTO UserPresence (GuildId, UserId, Timestamp, Presence, Game) values (?, ?, ?, ?, ?)", p.GuildID, p.User.ID, now.Format(time.RFC3339Nano), p.Status, gameName)
	if err != nil {
		fmt.Println("ERROR insert into UserPresence DB")
		fmt.Println(err.Error())
	}
}

func handleTypingStart(s *discordgo.Session, t *discordgo.TypingStart) {
	if t.UserID == ownUserID {
		return
	}
	if _, timerExists := typingTimer[t.UserID]; !timerExists && Rand.Intn(20) == 0 {
		typingTimer[t.UserID] = time.AfterFunc(20*time.Second, func() {
			responses := []string{"Something to say?", "Yes?", "Don't leave us hanging...", "I'm listening."}
			responseID := Rand.Intn(len(responses))
			s.ChannelMessageSend(t.ChannelID, fmt.Sprintf("<@%s> %s", t.UserID, responses[responseID]))
		})
	}
}

func handleVoiceUpdate(s *discordgo.Session, v *discordgo.VoiceStateUpdate) {
	_, err := sqlClient.Exec("INSERT INTO VoiceState (GuildId, ChanId, UserId, SessionId, Deaf, Mute, SelfDeaf, SelfMute, Suppress, Timestamp) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		v.GuildID, v.ChannelID, v.UserID, v.SessionID, v.Deaf, v.Mute, v.SelfDeaf, v.SelfMute, v.Suppress, time.Now().Format(time.RFC3339Nano))
	if err != nil {
		fmt.Println("ERROR insert into VoiceState: ", err.Error())
	}
}

func handleGuildMemberAdd(s *discordgo.Session, m *discordgo.GuildMemberAdd) {
	guild, err := s.Guild(m.GuildID)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	for _, channel := range guild.Channels {
		if channel.ID != minecraftChanID {
			s.ChannelMessageSend(channel.ID, fmt.Sprintf("*%s has joined*", m.User.Username))
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
		if channel.ID != minecraftChanID {
			s.ChannelMessageSend(channel.ID, fmt.Sprintf("*%s has left*", m.User.Username))
		}
	}
}

func tailMinecraftLog(session *discordgo.Session) {
	logTail := exec.Command("tail", "-F", "-n", "0", minecraftLogPath)
	logOut, err := logTail.StdoutPipe()
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	if err := logTail.Start(); err != nil {
		fmt.Println(err.Error())
		return
	}
	bufLogOut := bufio.NewScanner(logOut)
	logChan := make(chan string)
	go minecraftToDiscord(session, logChan)
	for bufLogOut.Scan() {
		logChan <- bufLogOut.Text()
	}
	if err := bufLogOut.Err(); err != nil {
		fmt.Println(err.Error())
	}
}

func minecraftToDiscord(session *discordgo.Session, logChan chan string) {
	joinedRegex := regexp.MustCompile(`(?i)^\[\d\d:\d\d:\d\d\] \[Server thread\/INFO\]: (\w+) joined the game$`)
	leftRegex := regexp.MustCompile(`(?i)^\[\d\d:\d\d:\d\d\] \[Server thread\/INFO\]: (\w+) left the game$`)
	chatRegex := regexp.MustCompile(`(?i)^\[\d\d:\d\d:\d\d\] \[Server thread\/INFO\]: <(\w+)> (.*)$`)
	serverChatRegex := regexp.MustCompile(`(?i)^\[\d\d:\d\d:\d\d\] \[Server thread\/INFO\]: \[Server\] (.*)$`)
	achievementRegex := regexp.MustCompile(`(?i)^\[\d\d:\d\d:\d\d\] \[Server thread\/INFO\]: (\w+ has just earned the achievement \[.*?\])$`)
	//deathRegex := regexp.MustCompile(`(?i)^\[\d\d:\d\d:\d\d\] \[Server thread\/INFO\]: (\w+) (?:was|walked|drowned|experienced|blew|hit|fell|went|walked|tried|got|starved|suffocated|withered)`)
	usernames := make(map[string]bool)

	for {
		select {
		case logLine := <-logChan:
			if match := chatRegex.FindStringSubmatch(logLine); match != nil {
				session.ChannelMessageSend(minecraftChanID, fmt.Sprintf("%s> %s", match[1], match[2]))
				break
			}
			if match := joinedRegex.FindStringSubmatch(logLine); match != nil {
				usernames[match[1]] = true
				session.ChannelMessageSend(minecraftChanID, fmt.Sprintf("*%s has joined*", match[1]))
				break
			}
			if match := leftRegex.FindStringSubmatch(logLine); match != nil {
				usernames[match[1]] = false
				session.ChannelMessageSend(minecraftChanID, fmt.Sprintf("*%s has left*", match[1]))
				break
			}
			if match := serverChatRegex.FindStringSubmatch(logLine); match != nil {
				session.ChannelMessageSend(minecraftChanID, fmt.Sprintf("[Server] %s", match[1]))
				break
			}
			if match := achievementRegex.FindStringSubmatch(logLine); match != nil {
				session.ChannelMessageSend(minecraftChanID, match[1])
				break
			}
			var otherRegexes []*regexp.Regexp
			for username, online := range usernames {
				if online {
					regex, err := regexp.Compile(`(?i)^\[\d\d:\d\d:\d\d\] \[Server thread\/INFO\]: ((` + username + `) .*)$`)
					if err != nil {
						fmt.Println(err.Error())
						break
					}
					otherRegexes = append(otherRegexes, regex)
				}
			}
			for _, regex := range otherRegexes {
				if match := regex.FindStringSubmatch(logLine); match != nil {
					lostConnectionRegex := regexp.MustCompile(`(?i)^\[\d\d:\d\d:\d\d\] \[Server thread\/INFO\]: ` + match[2] + ` lost connection`)
					if lostConnectionRegex.MatchString(logLine) {
						break
					}
					session.ChannelMessageSend(minecraftChanID, fmt.Sprintf("*%s*", match[1]))
					break
				}
			}
		}
	}
}

func normalizeKarma() {
	_, err := sqlClient.Exec("UPDATE UserKarma SET Karma = MAX(CAST(Karma / 3 AS INTEGER), 1)")
	if err != nil {
		fmt.Println(err.Error())
	}
	now := time.Now()
	nextRun := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.Local)
	time.AfterFunc(nextRun.Sub(now), normalizeKarma)
}

func main() {
	var err error
	sqlClient, err = sql.Open("sqlite3", "sqlite.db")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	client, err := discordgo.New(loginEmail, loginPassword)
	if err != nil {
		fmt.Println(err)
		return
	}
	client.StateEnabled = true

	self, err := client.User("@me")
	if err != nil {
		fmt.Println(err)
		return
	}
	ownUserID = self.ID

	client.AddHandler(makeMessageCreate())
	client.AddHandler(handlePresenceUpdate)
	//client.AddHandler(handleTypingStart)
	client.AddHandler(handleVoiceUpdate)
	client.AddHandler(handleGuildMemberAdd)
	client.AddHandler(handleGuildMemberRemove)
	client.Open()
	defer client.Close()
	defer client.Logout()
	defer func() {
		voiceMutex.Lock()
		defer voiceMutex.Unlock()
		if currentVoiceSession != nil {
			err := currentVoiceSession.Disconnect()
			if err != nil {
				fmt.Println("ERROR leaving voice channel " + err.Error())
			}
		}
	}()

	userGuilds, err := client.UserGuilds()
	if err != nil {
		fmt.Println("Error getting user guilds", err.Error())
	}
	tran, err := sqlClient.Begin()
	if err != nil {
		fmt.Println("Error starting transaction", err.Error())
	} else {
		now := time.Now()
		for _, guild := range userGuilds {
			guild, err = client.Guild(guild.ID)
			if err != nil {
				fmt.Println("Error getting guild", err.Error())
				continue
			}
			userMap := make(map[string]bool)
			for _, presence := range guild.Presences {
				gameName := ""
				if presence.Game != nil {
					gameName = presence.Game.Name
				}
				_, err := tran.Exec("INSERT INTO UserPresence (GuildId, UserId, Timestamp, Presence, Game) values (?, ?, ?, ?, ?)", guild.ID, presence.User.ID, now.Format(time.RFC3339Nano), presence.Status, gameName)
				if err != nil {
					fmt.Println("ERROR inserting into UserPresence DB", err.Error())
				}
				userMap[presence.User.ID] = true
			}
			for _, member := range guild.Members {
				if _, found := userMap[member.User.ID]; !found {
					_, err := tran.Exec("INSERT INTO UserPresence (GuildId, UserId, Timestamp, Presence, Game) values (?, ?, ?, ?, ?)", guild.ID, member.User.ID, now.Format(time.RFC3339Nano), "offline", "")
					if err != nil {
						fmt.Println("ERROR inserting into UserPresence DB", err.Error())
					}
				}
			}
		}
		tran.Commit()
	}

	signals := make(chan os.Signal, 1)

	go func() {
		select {
		case <-signals:
			voiceMutex.Lock()
			defer voiceMutex.Unlock()
			if currentVoiceSession != nil {
				err := currentVoiceSession.Disconnect()
				if err != nil {
					fmt.Println("ERROR leaving voice channel " + err.Error())
				}
			}
			client.Logout()
			client.Close()
			now := time.Now()
			for _, guild := range userGuilds {
				sqlClient.Exec("INSERT INTO UserPresence (GuildId, UserId, Timestamp, Presence, Game) values (?, ?, ?, ?, ?)", guild.ID, ownUserID, now.Format(time.RFC3339Nano), "offline", "")
			}
			os.Exit(0)
		}
	}()
	signal.Notify(signals, os.Interrupt)

	go initGameUpdater(client)

	now := time.Now()
	rows, err := sqlClient.Query("select ChanId, AuthorId, Time, Content from Reminder where Time > ?", now.In(time.FixedZone("UTC", 0)).Format(time.RFC3339))
	if err != nil {
		fmt.Println("ERROR setting reminders", err)
	}
	for rows.Next() {
		var chanID, authorID, timeStr, content string
		err := rows.Scan(&chanID, &authorID, &timeStr, &content)
		if err != nil {
			fmt.Println("ERROR setting reminder", err)
		}
		reminderTime, err := time.Parse(time.RFC3339, timeStr)
		if err != nil {
			fmt.Println("ERROR getting reminder time", err)
		}
		time.AfterFunc(reminderTime.Sub(now), func() { client.ChannelMessageSend(chanID, fmt.Sprintf("<@%s> %s", authorID, content)) })
	}
	rows.Close()

	if len(minecraftChanID) > 0 {
		go tailMinecraftLog(client)
	}

	nextRun := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.Local)
	time.AfterFunc(nextRun.Sub(now), normalizeKarma)

	var input string
	fmt.Scanln(&input)
	return
}
