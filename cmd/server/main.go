package main

import (
	"fmt"

	"github.com/bootdotdev/learn-pub-sub-starter/internal/gamelogic"
	"github.com/bootdotdev/learn-pub-sub-starter/internal/pubsub"
	"github.com/bootdotdev/learn-pub-sub-starter/internal/routing"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	//Connect to RabbitMQ
	fmt.Println("Starting Peril server...")
	connectionUrl := "amqp://guest:guest@mac.local:5672/"
	conn, err := amqp.Dial(connectionUrl)
	if err != nil {
		fmt.Println("Failed to connect to RabbitMQ:", err)
		return
	}
	defer conn.Close()

	channel, err := conn.Channel()
	if err != nil {
		fmt.Println("Failed to open a channel:", err)
		return
	}
	defer channel.Close()

	fmt.Println("Connected to RabbitMQ")

	////Declare and bind the game log queue
	// pubsub.DeclareAndBind(
	// 	conn,
	// 	routing.ExchangePerilTopic,
	// 	routing.GameLogSlug,
	// 	routing.GameLogSlug + ".*",
	// 	pubsub.SimpleQueueDurable)

	pubsub.SubscribeGob(
		conn, 
		routing.ExchangePerilTopic, 
		routing.GameLogSlug, 
		routing.GameLogSlug + ".*", 
		pubsub.SimpleQueueDurable, 
		handlerLog(),
	)

	for {
		gamelogic.PrintServerHelp()
		words := gamelogic.GetInput()

		if len(words) == 0 {
			continue
		}

		
		switch words[0] {
		case "pause":
			fmt.Println("Pausing the game...")
			err := pubsub.PublishJSON(channel, string(routing.ExchangePerilTopic), routing.PauseKey, routing.PlayingState{
				IsPaused: true,
			})
			if err != nil {
				fmt.Println("Failed to publish pause message:", err)
			}
		case "resume":
			fmt.Println("Resuming the game...")
			err := pubsub.PublishJSON(channel, string(routing.ExchangePerilTopic), routing.PauseKey, routing.PlayingState{
				IsPaused: false,
			})
			if err != nil {
				fmt.Println("Failed to publish resume message:", err)
			}
		case "quit":
			fmt.Println("Goodbye!")
			return
		default:
			fmt.Println("Unknown command:", words[0])

		}
		
	}
	

	
	

	
	// // wait for ctrl+c
	// signalChan := make(chan os.Signal, 1)
	// signal.Notify(signalChan, os.Interrupt)
	// <-signalChan
}

func handlerLog() func(routing.GameLog) pubsub.Acktype {
	return func(gl routing.GameLog) pubsub.Acktype {
		defer fmt.Print("> ")
		gamelogic.WriteLog(gl)
		return pubsub.Ack
	}
}

	