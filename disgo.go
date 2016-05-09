package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/bwmarrin/dgvoice"
	"github.com/bwmarrin/discordgo"
	"github.com/gyuho/goling/similar"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"image"
	imageColor "image/color"
	"image/png"
	"io/ioutil"
	"math"
	"math/rand"
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
	Karma  int64
}
type TwitchChannel struct {
	DisplayName string `json:"display_name"`
	Name        string `json:"name"`
	Status      string `json:"status"`
}
type TwitchStream struct {
	ID          int           `json:"_id"`
	AverageFps  float64       `json:"average_fps"`
	Game        string        `json:"game"`
	Viewers     int           `json:"viewers"`
	Channel     TwitchChannel `json:"channel"`
	VideoHeight int           `json:"video_height"`
}
type TwitchStreamReply struct {
	Stream *TwitchStream `json:"stream"`
}
type UserMessageCount struct {
	AuthorID    string
	NumMessages int64
}
type UserMessageLength struct {
	AuthorID  string
	AvgLength float64
}

type WolframPlaintextPod struct {
	Title     string `xml:"title,attr"`
	Error     bool   `xml:"error,attr"`
	Primary   *bool  `xml:"primary,attr"`
	Plaintext string `xml:"subpod>plaintext"`
}

type WolframQueryResult struct {
	Success bool                  `xml:"success,attr"`
	Error   bool                  `xml:"error,attr"`
	NumPods int                   `xml:"numpods,attr"`
	Pods    []WolframPlaintextPod `xml:"pod"`
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

var (
	sqlClient                       *sql.DB
	voteTime                        = make(map[string]time.Time)
	userIDRegex                     = regexp.MustCompile(`<@(\d+?)>`)
	typingTimer                     = make(map[string]*time.Timer)
	currentVoiceSession             *discordgo.VoiceConnection
	currentVoiceTimer               *time.Timer
	ownUserID                       = ""
	lastMessage, lastCommandMessage discordgo.Message
	lastAuthorID                    = ""
	voiceMutex                      sync.Mutex
	Rand                            = rand.New(rand.NewSource(time.Now().UnixNano()))
	lastQuoteIDs                    = make(map[string]int64)
	userIDUpQuotes                  = make(map[string][]string)
	userGuilds                      = make(map[string]discordgo.Guild)
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
	if maxUserID != "" {
		return maxUserID, nil
	}
	maxSim = 0.0
	maxUserID = ""
	if guild.Members != nil {
		for _, member := range guild.Members {
			if user := member.User; user != nil {
				sim := similar.Cosine([]byte(strings.ToLower(user.Username)), usernameBytes)
				if sim > maxSim {
					maxSim = sim
					maxUserID = user.ID
				}
			}
		}
	}
	if maxUserID == "" {
		return "", errors.New("No similar user found")
	}
	return maxUserID, nil
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

func vote(session *discordgo.Session, chanID, authorID, messageID string, args []string, inc int64) (string, error) {
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
	if authorID == userID && inc > 0 {
		_, err := vote(session, chanID, ownUserID, messageID, []string{"<@" + authorID + ">"}, -1)
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
	if lastVoterIDAgainstUser == userID && time.Since(lastVoteTime).Hours() < 12 {
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
	if lastVoteeIDFromAuthor == userID && time.Since(lastVoteTime).Hours() < 12 {
		return "Really?...", nil
	}

	var karma int64
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
	isUpvote := false
	if inc > 0 {
		isUpvote = true
	}
	_, err = sqlClient.Exec("insert into Vote(GuildId, MessageId, VoterID, VoteeID, Timestamp, IsUpvote) values (?, ?, ?, ?, ?, ?)",
		channel.GuildID, messageIDUnit, authorID, userID, time.Now().Format(time.RFC3339Nano), isUpvote)
	if err != nil {
		return "", err
	}

	return "", nil
}

func upvote(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	return vote(session, chanID, authorID, messageID, args, 1)
}

func downvote(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	return vote(session, chanID, authorID, messageID, args, -1)
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
		var karma int64
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
		finalString += fmt.Sprintf("%s — %d\n", user.Username, vote.Karma)
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
		var authorKarma int
		err = sqlClient.QueryRow("select Karma from UserKarma where GuildId = ? and UserId = ?", channel.GuildID, authorID).Scan(&authorKarma)
		if err != nil {
			authorKarma = 0
		}
		newLockedMinutes := Rand.Intn(30) + 45 + 10*authorKarma
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
			return fmt.Sprintf("%s's name change will last for an extra %d minutes thanks to their karma!", author.Username, 10*authorKarma), nil
		} else if authorKarma < 0 {
			return fmt.Sprintf("%s's name change will last up to %d minutes less due to their karma...", author.Username, -10*authorKarma), nil
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
	userID, err := getMostSimilarUserID(session, chanID, strings.Join(args, " "))
	if err != nil {
		return "", err
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
	userID, err := getMostSimilarUserID(session, chanID, strings.Join(args, " "))
	if err != nil {
		return "", err
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

func temp(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	output, err := exec.Command("sensors", "-f", "coretemp-isa-0000").Output()
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
		suh := Rand.Intn(28)
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
	atTimeRegex := regexp.MustCompile(`(?i)(?:at\s+)?(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}\s+[\+-]\d{4})\s+to\s+(.*)`)
	inTimeRegex := regexp.MustCompile(`(?i)(?:in)?\s*(?:(?:(?:(\d+)\s+years?)|(?:(\d+)\s+months?)|(?:(\d+)\s+weeks?)|(?:(\d+)\s+days?)|(?:(\d+)\s+hours?)|(?:(\d+)\s+minutes?)|(?:(\d+)\s+seconds?))\s?)+to\s+(.*)`)
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
	userID, err := getMostSimilarUserID(session, chanID, strings.Join(args, " "))
	if err != nil {
		return "", err
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
	userID, err := getMostSimilarUserID(session, chanID, strings.Join(args, " "))
	if err != nil {
		return "", err
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
	channel, err := session.State.Channel(chanID)
	if err != nil {
		return "", err
	}
	rows, err := sqlClient.Query("SELECT UserId, Timestamp, Game FROM UserPresence WHERE GuildId = ? ORDER BY Timestamp ASC", channel.GuildID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	userGame := make(map[string]string)
	userTime := make(map[string]time.Time)
	gameTime := make(map[string]float64)
	firstTime := time.Now()
	for rows.Next() {
		var userID, timestamp, game string
		err = rows.Scan(&userID, &timestamp, &game)
		if err != nil {
			return "", err
		}
		currTime, err := time.Parse(time.RFC3339Nano, timestamp)
		if err != nil {
			return "", err
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
	message := fmt.Sprintf("Since %s\n", firstTime.Format(time.RFC1123Z))
	for i := 0; i < limit && i < len(gameTimes); i++ {
		message += fmt.Sprintf("%"+strconv.Itoa(longestGameLength)+"s — %.2f\n", gameTimes[i].AuthorID, gameTimes[i].AvgLength)
	}
	return fmt.Sprintf("```%s```", message), nil
}

func help(session *discordgo.Session, chanID, authorID, messageID string, args []string) (string, error) {
	privateChannel, err := session.UserChannelCreate(authorID)
	if err != nil {
		return "", err
	}
	_, err = session.ChannelMessageSend(privateChannel.ID, `**asuh** - joins your voice channel
**age** [username] - displays how long [username] has been in this server
**ayy**
**bitrate** - shows voice channels and their bitrates
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
**playtime** [number (optional) - shows up to <number> summated (probably incorrect) playtimes in hours of every game across all users
**remindme**
	in [duration] to [x] - mentions user with <x> after <duration> (example: /remindme in 5 hours 10 minutes 3 seconds to order a pizza)
	at [time] to [x] - mentions user with <x> at <time> (example: /remindme at 2016-05-04 13:37:00 -0500 to make a clever xd facebook status)
**reminders** - messages you your pending reminders
**rename** [new username] - renames bot`)
	if err != nil {
		return "", err
	}
	_, err = session.ChannelMessageSend(privateChannel.ID, `**roll** [sides (optional)] - "rolls" a die with <sides> sides
**spam** [streamer (optional)] - generates a messages based on logs from <streamer>, shows all streamer logs if no streamer is specified
**spamdiscord** - generates a message based on logs from this discord channel
**spamuser** [username] - generates a message based on discord logs of <username>
**soda** - alias for /spam sodapoppin
**top** [number (optional)] - displays top <number> users sorted by messages sent
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
	oddshotRegex := regexp.MustCompile(`(?i)https?:\/\/(www\.)?oddshot.tv\/shot\/[\w-]+`)
	meanRegexes := []*regexp.Regexp{regexp.MustCompile(`(?i)fuc.*bot($|[[:space:]])`), regexp.MustCompile(`(?i)shit.*bot($|[[:space:]])`)}
	questionRegex := regexp.MustCompile(`^<@` + ownUserID + `>.*\w+.*\?$`)
	inTheChatRegex := regexp.MustCompile(`(?i)can i get a\s+(.*?)\s+in the chat`)
	funcMap := map[string]Command{
		"spam":        Command(spam),
		"soda":        Command(soda),
		"lirik":       Command(lirik),
		"forsen":      Command(forsen),
		"roll":        Command(roll),
		"help":        Command(help),
		"upvote":      Command(upvote),
		"downvote":    Command(downvote),
		"votes":       Command(votes),
		"karma":       Command(votes),
		"uptime":      Command(uptime),
		"twitch":      Command(twitch),
		"top":         Command(top),
		"toplength":   Command(topLength),
		"rename":      Command(rename),
		"lastseen":    Command(lastseen),
		"delete":      Command(deleteLastMessage),
		"cwc":         Command(cwc),
		"kickme":      Command(kickme),
		"spamuser":    Command(spamuser),
		"math":        Command(maths),
		"cputemp":     Command(temp),
		"ayy":         Command(ayy),
		"spamdiscord": Command(spamdiscord),
		"ping":        Command(ping),
		"xd":          Command(xd),
		"asuh":        Command(asuh),
		"upquote":     Command(upquote),
		"uq":          Command(upquote),
		"topquote":    Command(topquote),
		"8ball":       Command(eightball),
		"oddshot":     Command(oddshot),
		"remindme":    Command(remindme),
		"meme":        Command(meme),
		"bitrate":     Command(bitrate),
		"commands":    Command(help),
		"age":         Command(age),
		"lastmessage": Command(lastUserMessage),
		"reminders":   Command(reminders),
		"color":       Command(color),
		"playtime":    Command(playtime),
		string([]byte{119, 97, 116, 99, 104, 108, 105, 115, 116}): Command(wlist),
	}

	executeCommand := func(s *discordgo.Session, m *discordgo.MessageCreate, command []string) {
		if cmd, valid := funcMap[strings.ToLower(command[0])]; valid {
			if command[0] != "upvote" &&
				command[0] != "downvote" &&
				command[0] != "help" &&
				command[0] != "commands" &&
				command[0] != "rename" &&
				command[0] != "delete" &&
				command[0] != "asuh" &&
				command[0] != "uq" &&
				command[0] != "upquote" &&
				command[0] != "reminders" {
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

		if typingTimer, valid := typingTimer[m.Author.ID]; valid {
			typingTimer.Stop()
		}

		if strings.Contains(strings.ToLower(m.Content), "vape") || strings.Contains(strings.ToLower(m.Content), "v/\\") || strings.Contains(strings.ToLower(m.Content), "\\//\\") || strings.Contains(strings.ToLower(m.Content), "\\\\//\\") {
			s.ChannelMessageSend(m.ChannelID, "🆅🅰🅿🅴 🅽🅰🆃🅸🅾🅽")
		}
		for _, meanRegex := range meanRegexes {
			if match := meanRegex.FindString(m.Content); match != "" {
				respond := Rand.Intn(3)
				if respond == 0 {
					responses := []string{":(", "ayy fuck you too", "asshole.", "<@" + m.Author.ID + "> --"}
					_, err := s.ChannelMessageSend(m.ChannelID, responses[Rand.Intn(len(responses))])
					if err != nil {
						fmt.Println("Error sending response " + err.Error())
					}
					break
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
		if match := oddshotRegex.FindString(m.Content); match != "" {
			executeCommand(s, m, []string{"oddshot", match})
			return
		}
		for _, regex := range regexes {
			if match := regex.FindStringSubmatch(m.Content); match != nil {
				executeCommand(s, m, strings.Fields(match[1]))
				return
			}
		}
	}
}

func gameUpdater(s *discordgo.Session, ticker <-chan time.Time) {
	currentGame := ""
	games := []string{"Skynet Simulator 2020", "Kill All Humans", "WW III: The Game", "9GAG Meme Generator", "Subreddit Simulator",
		"Runescape", "War Games", "Half Life 3", "Secret of the Magic Crystals", "Dransik", "<Procedurally Generated Name>",
		"Call of Duty 3", "Dino D-Day", "Overwatch", "Euro Truck Simulator 2", "Farmville", "Dwarf Fortress",
		"Pajama Sam: No Need to Hide When It's Dark Outside", "League of Legends", "The Ship", "Sleepy Doge", "Surgeon Simulator",
		"Farming Simulator 2018: The Farming"}
	for {
		select {
		case <-ticker:
			if currentGame != "" {
				changeGame := Rand.Intn(3)
				if changeGame != 0 {
					continue
				}
				currentGame = ""
			} else {
				index := Rand.Intn(len(games) * 5)
				if index >= len(games) {
					currentGame = ""
				} else {
					currentGame = games[index]
				}
			}
			err := s.UpdateStatus(0, currentGame)
			if err != nil {
				fmt.Println("ERROR updating game: ", err.Error())
			}
		}
	}
}

func kickChecker(updateUserGuilds func() ([]*discordgo.Guild, error), ticker <-chan time.Time) {
	for {
		select {
		case <-ticker:
			newGuilds := make(map[string]discordgo.Guild)
			guilds, err := updateUserGuilds()
			if err != nil {
				fmt.Println("Error getting userGuilds ", err.Error())
			}
			for _, guild := range guilds {
				newGuilds[guild.ID] = *guild
			}
			for guildID := range userGuilds {
				if _, found := newGuilds[guildID]; !found {
					res, err := http.PostForm("http://textbelt.com/text", url.Values{"number": {phoneNumber}, "message": {"Kicked from " + userGuilds[guildID].Name}})
					if err != nil {
						fmt.Println("Error sending SMS", err.Error())
					}
					defer res.Body.Close()
					if res.StatusCode != 200 {
						fmt.Println("Error sending SMS status:", res.Status)
						body, err := ioutil.ReadAll(res.Body)
						if err != nil {
							fmt.Println("Error reading response body", err.Error())
						}
						fmt.Println(body)
					}
				}
			}
			userGuilds = newGuilds
		}
	}
}

func handlePresenceUpdate(s *discordgo.Session, p *discordgo.PresenceUpdate) {
	now := time.Now()
	if p.User == nil {
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
	client.AddHandler(handleTypingStart)
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
	userGuildsArr, err := client.UserGuilds()
	if err != nil {
		fmt.Println("Error getting user guilds", err.Error())
	}
	for _, guild := range userGuildsArr {
		userGuilds[guild.ID] = *guild
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
			os.Exit(0)
		}
	}()
	signal.Notify(signals, os.Interrupt)

	gameTicker := time.NewTicker(817 * time.Second)
	go gameUpdater(client, gameTicker.C)

	kickCheckTicker := time.NewTicker(5 * time.Minute)
	go kickChecker(client.UserGuilds, kickCheckTicker.C)

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

	var input string
	fmt.Scanln(&input)
	return
}
