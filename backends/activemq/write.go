package activemq

import (
	"github.com/jhump/protoreflect/desc"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/batchcorp/plumber/cli"
	"github.com/batchcorp/plumber/writer"
)

func Write(opts *cli.Options, md *desc.MessageDescriptor) error {
	if err := writer.ValidateWriteOptions(opts, nil); err != nil {
		return errors.Wrap(err, "unable to validate write options")
	}

	writeValues, err := writer.GenerateWriteValues(md, opts)
	if err != nil {
		return errors.Wrap(err, "unable to generate write value")
	}

	client, err := NewClient(opts)
	if err != nil {
		return errors.Wrap(err, "unable to create client")
	}

	a := &ActiveMq{
		Options: opts,
		MsgDesc: md,
		Client:  client,
		log:     logrus.WithField("pkg", "activemq/write.go"),
	}

	for _, value := range writeValues {
		if err := a.Write(value); err != nil {
			a.log.Error(err)
		}
	}

	return nil
}

// Write writes a message to an ActiveMQ topic
func (a *ActiveMq) Write(value []byte) error {

	err := a.Client.Send(a.getDestination(), "", value, nil)
	if err != nil {
		a.log.Infof("Unable to write message to '%s': %s", a.getDestination(), err)
		return errors.Wrap(err, "unable to write message")
	}

	if err := a.Client.Disconnect(); err != nil {
		return errors.Wrap(err, "unable to disconnect nicely from activemq server")
	}

	a.log.Infof("Successfully wrote message to '%s'", a.getDestination())

	return nil
}
