package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jrh3k5/rich-go/client"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewDevelopment()

	if len(os.Args) < 2 {
		logger.Fatal("A Discord application ID must be provided")
	}

	defer func() {
		client.Logout()
	}()

	clientID := os.Args[1]
	logger.Info("Using client ID: " + clientID)

	if loginErr := client.Login(clientID); loginErr != nil {
		logger.Fatal("Unable to log in", zap.Error(loginErr))
	}

	logger.Info("Starting HTTP server...")

	if serverErr := runServer(logger, clientID); serverErr != nil {
		logger.Fatal("Failed to run HTTP server to intercept webhook calls", zap.Error(serverErr))
	}
}

func runServer(logger *zap.Logger, clientID string) error {
	http.HandleFunc("/", newHandleWebhookCall(logger, clientID))
	return http.ListenAndServe("0.0.0.0:9843", nil)
}

func newHandleWebhookCall(logger *zap.Logger, clientID string) func(http.ResponseWriter, *http.Request) {
	return func(responseWriter http.ResponseWriter, request *http.Request) {
		defer func() {
			_ = request.Body.Close()
		}()

		bodyBytes, err := io.ReadAll(request.Body)
		if err != nil {
			logger.Error("Failed to read body", zap.Error(err))
			responseWriter.WriteHeader(http.StatusInternalServerError)
			return
		}

		if len(bodyBytes) == 0 {
			logger.Warn("Request body was empty; no Discord status change will happen")
			responseWriter.WriteHeader(http.StatusBadRequest)
			return
		}

		logger.Sugar().Debugf("Request body bytes = %s", string(bodyBytes))

		var payload *playbackPayload
		if unmarshalErr := json.Unmarshal(bodyBytes, &payload); unmarshalErr != nil {
			logger.Error(fmt.Sprintf("Failed to unmarshal %d JSON bytes", len(bodyBytes)), zap.Error(unmarshalErr))
			responseWriter.WriteHeader(http.StatusInternalServerError)
			return
		}

		var actors []string
		for _, actorName := range strings.Split(payload.Actors, ",") {
			trimmedName := strings.TrimSpace(actorName)
			if len(trimmedName) == 0 {
				continue
			}

			actors = append(actors, actorName)
		}

		var secondsRemaining int
		if parsedInt, parseErr := strconv.ParseInt(payload.SecondsRemaining, 10, 64); parseErr != nil {
			logger.Warn(fmt.Sprintf("Unable to parse seconds remaining value of '%s'; the time remaining will not be calculated", payload.SecondsRemaining), zap.Error(parseErr))
		} else {
			secondsRemaining = int(parsedInt)
		}

		timeLeft := time.Duration(secondsRemaining) * time.Second

		if setStatusErr := setDiscordStatus(logger, clientID, payload.Title, payload.Studio, actors, timeLeft, 0); setStatusErr != nil {
			logger.Error("Failed to set Discord status", zap.Error(setStatusErr))
			responseWriter.WriteHeader(http.StatusInternalServerError)
			return
		}

		responseWriter.WriteHeader(http.StatusOK)
	}
}

func setDiscordStatus(logger *zap.Logger, clientID string, title string, studioName string, actors []string, timeRemaining time.Duration, tryAttempt int) error {
	var state string
	if len(actors) > 0 {
		if len(actors) <= 3 {
			copyLength := 3
			if copyLength > len(actors) {
				copyLength = len(actors)
			}
			actorsCopy := make([]string, copyLength)
			copy(actorsCopy, actors)
			sort.Strings(actorsCopy)
			state = "Starring " + strings.Join(actorsCopy, ", ")
		} else {
			state = fmt.Sprintf("Starring %d actors", len(actors))
		}
	}

	var details string
	if studioName != "" {
		details = fmt.Sprintf("(%s) %s", studioName, title)
	} else {
		details = title
	}

	now := time.Now()
	endTime := now.Add(timeRemaining)
	if activityErr := client.SetActivity(client.Activity{
		State:   state,
		Details: details,
		Timestamps: &client.Timestamps{
			Start: &now,
			End:   &endTime,
		},
	}); activityErr != nil {
		logger.Warn("Failed to set activity", zap.Error(activityErr))

		if tryAttempt >= 1 {
			return fmt.Errorf("failed to set activity; retries exhausted (%d >= 1): %w", tryAttempt, activityErr)
		}

		if logoutErr := client.Logout(); logoutErr != nil {
			logger.Warn("Failed to log out prior to retry setting activity", zap.Error(logoutErr))
		}

		if loginErr := client.Login(clientID); loginErr != nil {
			return fmt.Errorf("failed to log in prior to retrying setting activity: %w", loginErr)
		}

		return setDiscordStatus(logger, clientID, title, studioName, actors, timeRemaining, tryAttempt+1)
	}

	return nil
}

type playbackPayload struct {
	Title            string `json:"title"`
	Actors           string `json:"actors"`
	Studio           string `json:"studio"`
	SecondsRemaining string `json:"secondsRemaining"`
}
