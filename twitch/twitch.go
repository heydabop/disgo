package twitch

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
)

type usersResponse struct {
	Users []struct {
		ID *string `json:"_id"`
	} `json:"users"`
}

type streamsResponse struct {
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

// StreamInfo returns a formatted string containing a stream name, status, viewer count, and video quality
func StreamInfo(channel, clientID string) (string, error) {
	client := http.Client{}

	userReq, err := http.NewRequest("GET", fmt.Sprintf("https://api.twitch.tv/kraken/users?login=%s", url.QueryEscape(channel)), nil)
	if err != nil {
		return "", err
	}
	userReq.Header.Add("Client-ID", clientID)
	userReq.Header.Add("Accept", "application/vnd.twitchtv.v5+json")
	userRes, err := client.Do(userReq)
	if err != nil {
		return "", err
	}
	defer userRes.Body.Close()
	if userRes.StatusCode != 200 {
		return "", errors.New(userRes.Status)
	}
	userBody, err := ioutil.ReadAll(userRes.Body)
	if err != nil {
		return "", err
	}
	var users usersResponse
	if err := json.Unmarshal(userBody, &users); err != nil {
		return "", err
	}
	if users.Users == nil || len(users.Users) < 1 || users.Users[0].ID == nil {
		return "", errors.New("not found")
	}

	streamReq, err := http.NewRequest("GET", fmt.Sprintf("https://api.twitch.tv/kraken/streams/%s", url.QueryEscape(*users.Users[0].ID)), nil)
	if err != nil {
		return "", err
	}
	streamReq.Header.Add("Client-ID", clientID)
	streamReq.Header.Add("Accept", "application/vnd.twitchtv.v5+json")
	streamRes, err := client.Do(streamReq)
	if err != nil {
		return "", err
	}
	defer streamRes.Body.Close()
	if streamRes.StatusCode != 200 {
		return "", errors.New(streamRes.Status)
	}
	streamBody, err := ioutil.ReadAll(streamRes.Body)
	if err != nil {
		return "", err
	}
	var stream streamsResponse
	if err = json.Unmarshal(streamBody, &stream); err != nil {
		return "", err
	}
	if stream.Stream == nil {
		return "", nil
		//return "[Offline]", nil
	}
	return fmt.Sprintf(`%s playing %s
%s
%d viewers; %dp @ %.f FPS`, stream.Stream.Channel.Name, stream.Stream.Game, stream.Stream.Channel.Status, stream.Stream.Viewers, stream.Stream.VideoHeight, math.Floor(stream.Stream.AverageFps+0.5)), nil
}
