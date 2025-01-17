package nsq

import (
	"github.com/jhump/protoreflect/desc"
	"github.com/nsqio/go-nsq"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/batchcorp/plumber/cli"
	"github.com/batchcorp/plumber/writer"
)

// Write performs necessary setup and calls NSQ.Write() to write the actual message
func Write(opts *cli.Options, md *desc.MessageDescriptor) error {
	if err := writer.ValidateWriteOptions(opts, validateWriteOptions); err != nil {
		return errors.Wrap(err, "unable to validate write options")
	}

	writeValues, err := writer.GenerateWriteValues(md, opts)
	if err != nil {
		return errors.Wrap(err, "unable to generate write value")
	}

	logger := &NSQLogger{}
	logger.Entry = logrus.WithField("pkg", "nsq")

	n := &NSQ{
		Options: opts,
		MsgDesc: md,
		log:     logger,
	}

	for _, value := range writeValues {
		if err := n.Write(value); err != nil {
			n.log.Error(err)
		}
	}

	return nil
}

// Write publishes a message to a NSQ topic
func (n *NSQ) Write(value []byte) error {
	config, err := getNSQConfig(n.Options)
	if err != nil {
		return errors.Wrap(err, "unable to create NSQ config")
	}

	producer, err := nsq.NewProducer(n.Options.NSQ.NSQDAddress, config)
	if err != nil {
		return errors.Wrap(err, "unable to start NSQ producer")
	}

	logLevel := nsq.LogLevelError
	if n.Options.Debug {
		logLevel = nsq.LogLevelDebug
	}

	// Use logrus for NSQ logs
	producer.SetLogger(n.log, logLevel)

	defer producer.Stop()

	err = producer.Publish(n.Options.NSQ.Topic, value)
	if err != nil {
		return errors.Wrap(err, "unable to publish message to NSQ")
	}

	n.log.Infof("Successfully wrote message to '%s'", n.Options.NSQ.Topic)
	return nil
}

func validateWriteOptions(opts *cli.Options) error {
	if opts.NSQ.TLSCAFile != "" || opts.NSQ.TLSClientCertFile != "" || opts.NSQ.TLSClientKeyFile != "" {
		if opts.NSQ.TLSClientKeyFile == "" {
			return ErrMissingTLSKey
		}

		if opts.NSQ.TLSClientCertFile == "" {
			return ErrMissingTlsCert
		}

		if opts.NSQ.TLSCAFile == "" {
			return ErrMissingTLSCA
		}
	}

	return nil
}
