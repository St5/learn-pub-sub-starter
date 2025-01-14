package pubsub

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Acktype int

type SimpleQueueType int

const (
	SimpleQueueDurable SimpleQueueType = iota
	SimpleQueueTransient
)

const (
	Ack Acktype = iota
	NackRequeue
	NackDiscard
)	


/**
 * PublishJSON publishes a JSON-encoded message to the specified exchange and key.
 */
func PublishJSON[T any](ch *amqp.Channel, exchange, key string, val T) error {
	data, err := json.Marshal(val)
	if err != nil {
		return err
	}
	ch.PublishWithContext(context.Background(), exchange, key, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        data,
	})

	return nil
}

/**
* Publish object to a queue and serialize it using gob
**/
func PublishGob[T any](ch *amqp.Channel, exchange, key string, val T) error {
	var data bytes.Buffer
	enc := gob.NewEncoder(&data)

	err := enc.Encode(val)
	if err != nil {
		return err
	}
	err = ch.PublishWithContext(context.Background(), exchange, key, false, false, amqp.Publishing{
		ContentType: "application/gob",
		Body:        data.Bytes(),
	})
	if err != nil {
		return err
	}
	return nil
}

/**
 * DeclareAndBind declares an exchange, declares a queue, binds the queue to the exchange, and returns the channel and queue.
 */
func DeclareAndBind(
	conn *amqp.Connection,
	exchange,
	queueName,
	key string,
	simpleQueueType SimpleQueueType, //TODO with it
) (*amqp.Channel, amqp.Queue, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, amqp.Queue{}, err
	}

	queue, err := ch.QueueDeclare(
		queueName,
		simpleQueueType == SimpleQueueDurable,
		simpleQueueType != SimpleQueueDurable,
		simpleQueueType != SimpleQueueDurable,
		false,
		amqp.Table{
			"x-dead-letter-exchange": "peril_dlx",
		},
	)
	if err != nil {
		return nil, amqp.Queue{}, err
	}

	err = ch.QueueBind(queue.Name, key, exchange, false, nil)
	if err != nil {
		return nil, amqp.Queue{}, err
	}

	return ch, queue, nil
}

/**
 * SubscribeJSON subscribes to the specified exchange and key, and calls the handler function for each message received.
 */
func SubscribeJSON[T any](
	conn *amqp.Connection,
	exchange,
	queueName,
	key string,
	simpleQueueType SimpleQueueType,
	handler func(T) Acktype,
) error {
	ch, queue, err := DeclareAndBind(conn, exchange, queueName, key, simpleQueueType)
	if err != nil {
		return err
	}
	delivery, err := ch.Consume(queue.Name, "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	unmarshaller := func(data []byte) (T, error) {
		var target T
		err := json.Unmarshal(data, &target)
		return target, err
	}

	go func() {
		defer ch.Close()
		for msg := range delivery {
			target, err := unmarshaller(msg.Body)
			if err != nil {
				fmt.Println("Failed to unmarshal message:", err)	
			} 
			ack := handler(target)
			switch ack {
			case Ack:				
				msg.Ack(false)
				fmt.Println("Acked")
			case NackRequeue:
				msg.Nack(false, true)
				fmt.Println("NackRequeue")
			case NackDiscard:
				msg.Nack(false, false)
				fmt.Println("NackDiscard")
			default:
				fmt.Println("Unknown ack type:", ack)
			}
		}
	}()	
	return nil
}


func SubscribeGob[T any](
	conn *amqp.Connection,
	exchange,
	queueName,
	key string,
	simpleQueueType SimpleQueueType,
	handler func(T) Acktype,
) error {
	ch, queue, err := DeclareAndBind(conn, exchange, queueName, key, simpleQueueType)
	if err != nil {
		return err
	}
	ch.Qos(10, 0, false)
	delivery, err := ch.Consume(queue.Name, "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	decode := func(data []byte) (T, error) {
		buf := bytes.NewBuffer(data)
		dec := gob.NewDecoder(buf)
		
		var v T
		err := dec.Decode(&v)
		return v, err
	}

	go func() {
		defer ch.Close()
		for msg := range delivery {
			target, err := decode(msg.Body)
			if err != nil {
				fmt.Println("Failed to unmarshal message:", err)	
			} 
			ack := handler(target)
			switch ack {
			case Ack:				
				msg.Ack(false)
				fmt.Println("Acked")
			case NackRequeue:
				msg.Nack(false, true)
				fmt.Println("NackRequeue")
			case NackDiscard:
				msg.Nack(false, false)
				fmt.Println("NackDiscard")
			default:
				fmt.Println("Unknown ack type:", ack)
			}
		}
	}()	
	return nil
}
