package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/rs/zerolog/pkgerrors"
	"github.com/tidwall/gjson"
	"github.com/urfave/cli/v2"
)

var (
	client = http.Client{
		Timeout: 5 * time.Second,
	}
)

func graphQL(clientID, s string) ([]byte, error) {
	req, err := http.NewRequest("POST", "https://gql.twitch.tv/gql", strings.NewReader(s))

	if err != nil {
		return nil, err
	}

	req.Header.Set("Client-ID", clientID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)

	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusOK {
		return io.ReadAll(resp.Body)
	}

	return nil, nil
}

func fetchVideoComments(clientID, videoID string, after string) (string, []any, error) {
	s := fmt.Sprintf(
		`{"operationName":"VideoCommentsByOffsetOrCursor","variables":{"videoID":"%s","cursor":"%s"},"extensions":{"persistedQuery":{"version":1,"sha256Hash":"b70a3591ff0f4e0313d126c6a1502d79a1c02baebb288227c582044aa76adf6a"}}}`,
		videoID,
		after,
	)

	json, err := graphQL(clientID, s)

	if err != nil {
		return "", nil, err
	}

	var cursor string
	var comments []any

	gjson.GetBytes(json, "data.video.comments.edges").
		ForEach(func(key, value gjson.Result) bool {
			cursor = value.Get("cursor").
				String()

			comments = append(
				comments,

				value.Get("node").
					Value(),
			)

			return true
		})

	return cursor, comments, nil
}

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack

	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out: os.Stdout,
	})

	app := cli.NewApp()

	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:     "client-id",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "video-id",
			Required: true,
		},
		&cli.StringFlag{
			Name:  "output",
			Value: "output.json",
		},
		&cli.IntFlag{
			Name:  "delay",
			Value: 1000,
		},
	}

	app.Action = func(ctx *cli.Context) error {
		clientID := ctx.String("client-id")
		videoID := ctx.String("video-id")
		output := ctx.String("output")
		delay := ctx.Int("delay")

		var after string
		var history []any

		for {
			log := log.With().
				Str("video_id", videoID).
				Logger()

			log.Debug().
				Str("after", after).
				Msg("Fetching video comments...")

			cursor, comments, err := fetchVideoComments(clientID, videoID, after)

			if err != nil {
				log.Err(err).
					Msg("An error occured while fetching video comments")
			}

			history = append(history, comments...)
			after = cursor

			if after == "" {
				break
			}

			time.Sleep(time.Duration(delay) * time.Millisecond)
		}

		file, err := os.OpenFile(output, os.O_CREATE|os.O_TRUNC, os.ModePerm)

		if err != nil {
			log.Err(err).
				Msg("An error occured while opening file")
		}

		if err := json.NewEncoder(file).Encode(history); err != nil {
			log.Err(err).
				Msg("An error occured while writing file")
		}

		return nil
	}

	app.Run(os.Args)
}
