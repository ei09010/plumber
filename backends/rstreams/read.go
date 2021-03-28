package rstreams

import (
	"context"
	"fmt"

	"github.com/go-redis/redis/v8"
	"github.com/jhump/protoreflect/desc"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/batchcorp/plumber/cli"
	"github.com/batchcorp/plumber/pb"
	"github.com/batchcorp/plumber/printer"
	"github.com/batchcorp/plumber/reader"
)

func Read(opts *cli.Options) error {
	var mdErr error
	var md *desc.MessageDescriptor

	if opts.ReadProtobufRootMessage != "" {
		md, mdErr = pb.FindMessageDescriptor(opts.ReadProtobufDirs, opts.ReadProtobufRootMessage)
		if mdErr != nil {
			return errors.Wrap(mdErr, "unable to find root message descriptor")
		}
	}

	client, err := NewClient(opts)
	if err != nil {
		return errors.Wrap(err, "unable to create client")
	}

	r := &RedisStreams{
		Options: opts,
		Client:  client,
		MsgDesc: md,
		Context: context.Background(),
		log:     logrus.WithField("pkg", "rstreams/read.go"),
	}

	streamsOpts := r.Options.RedisStreams

	// Create consumer group for each stream
	for _, stream := range r.Options.RedisStreams.Streams {
		if r.Options.RedisStreams.RecreateConsumerGroup {
			r.log.Debugf("deleting consumer group '%s'", streamsOpts.ConsumerGroup)

			_, err := client.XGroupDestroy(r.Context, stream, streamsOpts.ConsumerGroup).Result()
			if err != nil {
				return fmt.Errorf("unable to reset consumer group: %s", err)
			}
		}

		r.log.Debugf("Creating stream with start id '%s'", streamsOpts.StartID)

		_, err := client.XGroupCreate(r.Context, stream, streamsOpts.ConsumerGroup, streamsOpts.StartID).Result()
		if err != nil {
			// No problem if consumer group already exists
			if err.Error() != "BUSYGROUP Consumer Group name already exists" {
				return fmt.Errorf("error creating consumer group for stream '%s': %s", stream, err)
			}
		}
	}

	return r.Read()
}

func generateStreams(streams []string) []string {
	for i := 0; i != len(streams); i++ {
		streams = append(streams, ">")
		i++
	}

	return streams
}

func (r *RedisStreams) Read() error {
	defer r.Client.Close()

	streams := generateStreams(r.Options.RedisStreams.Streams)

	r.log.Info("Listening for message(s) ...")

	lineNumber := 1

	for {
		// Attempt to consume
		streamsResult, err := r.Client.XReadGroup(r.Context, &redis.XReadGroupArgs{
			Group:    r.Options.RedisStreams.ConsumerGroup,
			Consumer: r.Options.RedisStreams.ConsumerName,
			Streams:  streams,
			Count:    r.Options.RedisStreams.Count,
			Block:    0,
			NoAck:    false,
		}).Result()

		if err != nil {
			return fmt.Errorf("unable to read from streamsResult: %s", err)
		}

		// We may be reading from multiple streamsResult - read each stream resp
		for _, stream := range streamsResult {
			streamName := stream.Stream

			// Each stream result may contain multiple messages
			for _, message := range stream.Messages {
				// A single message may contain multiple kv's
				for k, v := range message.Values {
					bytesData, ok := v.(string)
					if !ok {
						r.log.Errorf("[ID: %s Stream: %s Key: %s] unable to type assert value as string: %s; skipping",
							message.ID, streamName, k, err)

						continue
					}

					decodedData, err := reader.Decode(r.Options, r.MsgDesc, []byte(bytesData))
					if err != nil {
						r.log.Errorf("[ID: %s Stream: %s Key: %s] unable to decode message: %s; skipping",
							message.ID, streamName, k, err)
						continue
					}

					str := fmt.Sprintf("[ID: %s Stream: %s Key: %s] %s", message.ID, streamName, k, string(decodedData))

					if r.Options.ReadLineNumbers {
						str = fmt.Sprintf("%d: %s", lineNumber, str)
						lineNumber++
					}

					printer.Print(str)
				}
			}
		}

		if !r.Options.ReadFollow {
			return nil
		}
	}
}
