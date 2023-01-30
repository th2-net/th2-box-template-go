package component

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"syscall"
	p_buff "th2-grpc/th2_grpc_common"

	"github.com/rs/zerolog/log"
	rabbitmq "github.com/th2-net/th2-common-go/schema/modules/mqModule"
	"github.com/th2-net/th2-common-go/schema/queue/MQcommon"
	timestamp "google.golang.org/protobuf/types/known/timestamppb"
)

type MessageTypeListener struct {
	MessageType    string
	Function       func(args ...interface{})
	RootEventID    *p_buff.EventID
	Module         *rabbitmq.RabbitMQModule
	AmountReceived int
	NBatches       int
	Stats          map[string]int
	Wait           <-chan bool
	Ch             chan os.Signal
}

func NewListener(RootEventID *p_buff.EventID, module *rabbitmq.RabbitMQModule, BoxConf *BoxConfiguration, wait <-chan bool, ch chan os.Signal, Function func(args ...interface{})) *MessageTypeListener {
	return &MessageTypeListener{
		MessageType:    BoxConf.MessageType,
		Function:       Function,
		RootEventID:    RootEventID,
		Module:         module,
		AmountReceived: 0,
		NBatches:       4,
		Wait:           wait,
		Ch:             ch,
		Stats:          make(map[string]int),
	}
}

func (listener *MessageTypeListener) Handle(delivery *MQcommon.Delivery, batch *p_buff.MessageGroupBatch) error {

	defer func() {
		if r := recover(); r != nil {
			log.Err(fmt.Errorf("%v", r)).Msg("Error occurred while processing the received message.")
			listener.Ch <- syscall.SIGINT
			<-listener.Wait
		}
		listener.AmountReceived += 1
		if listener.AmountReceived%listener.NBatches == 0 {
			log.Log().Msg("Sending Statistic Event")
			table := GetNewTable("Message Type", "Amount")
			table.AddRow("Raw_Message", fmt.Sprint(listener.Stats["Raw"]))
			table.AddRow("Message", fmt.Sprint(listener.Stats["Messsage"]))
			var payloads []Table
			payloads = append(payloads, *table)
			encoded, _ := json.Marshal(&payloads)
			listener.Module.MqEventRouter.SendAll(CreateEventBatch(
				listener.RootEventID, CreateEvent(
					CreateEventID(), listener.RootEventID, timestamp.Now(), timestamp.Now(), 0, "Statistic on Batches", "message", encoded, nil),
			), "publish")
		}
	}()

	for _, group := range batch.Groups {
		for _, AnyMessage := range group.Messages {
			switch AnyMessage.GetKind().(type) {
			case *p_buff.AnyMessage_RawMessage:
				log.Log().Msg("Received Raw Message")
				listener.Stats["Raw"] += 1
			case *p_buff.AnyMessage_Message:
				log.Log().Msg("Received Message")
				listener.Stats["Message"] += 1
				msg := AnyMessage.GetMessage()
				if msg.Metadata == nil {
					listener.Module.MqEventRouter.SendAll(CreateEventBatch(
						listener.RootEventID, CreateEvent(
							CreateEventID(), listener.RootEventID, timestamp.Now(), timestamp.Now(), 0, "Error: metadata not set", "message", nil, nil),
					), "publish")
					log.Err(errors.New("nil metadata")).Msg("Metadata not set for the message")
				} else if msg.Metadata.MessageType == listener.MessageType {
					log.Log().Msgf("Received message with %v message type\n", listener.MessageType)
					listener.Function()
					log.Log().Msg("Triggered the function")
				}
			}
		}
	}

	return nil
}

func (listener MessageTypeListener) OnClose() error {
	log.Info().Msg("Listener OnClose")
	return nil
}
