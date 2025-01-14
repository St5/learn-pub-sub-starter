package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/bootdotdev/learn-pub-sub-starter/internal/gamelogic"
	"github.com/bootdotdev/learn-pub-sub-starter/internal/pubsub"
	"github.com/bootdotdev/learn-pub-sub-starter/internal/routing"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	//Connect to RabbitMQ
	fmt.Println("Starting Peril client...")
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

	//Get USERNAME
	username, err := gamelogic.ClientWelcome()
	if err != nil {
		fmt.Println(err)
		return
	}

	// _, queue, err := pubsub.DeclareAndBind(
	// 	conn,
	// 	routing.ExchangePerilTopic,
	// 	routing.PauseKey+"."+username,
	// 	routing.PauseKey,
	// 	pubsub.SimpleQueueTransient,
	// )
	// if err != nil {
	// 	fmt.Println("Failed to declare and bind queue:", err)
	// 	return
	// }
	// fmt.Printf("Queue %s declared and bound\n", queue.Name)

	gamestate := gamelogic.NewGameState(username)
	//Subscribe to pause messages
	err = pubsub.SubscribeJSON(
		conn,
		routing.ExchangePerilTopic,
		routing.PauseKey+"."+username,
		routing.PauseKey,
		pubsub.SimpleQueueTransient,
		handlerPause(gamestate),
	)
	if err != nil {
		fmt.Println("Failed to subscribe to pause messages:", err)
		return
	}
	//Subscribe to army_moves messages
	err = pubsub.SubscribeJSON(
		conn,
		routing.ExchangePerilTopic,
		routing.ArmyMovesPrefix+"."+username,
		routing.ArmyMovesPrefix+".*",
		pubsub.SimpleQueueTransient,
		handleArmyMoves(gamestate, channel),
	)
	if err != nil {
		fmt.Println("Failed to subscribe to army_moves messages:", err)
		return
	}
	err = pubsub.SubscribeJSON(
		conn,
		string(routing.ExchangePerilTopic),
		string(routing.WarRecognitionsPrefix),
		string(routing.WarRecognitionsPrefix+".*"),
		pubsub.SimpleQueueDurable,
		handlerWar(gamestate, channel),
	)
	for {
		fmt.Print("> ")
		words := gamelogic.GetInput()
		if len(words) == 0 {
			continue
		}

		switch words[0] {
		case "spawn":
			err := gamestate.CommandSpawn(words)
			if err != nil {
				fmt.Println(err)
			}
		case "move":
			mover, err := gamestate.CommandMove(words)
			if err != nil {
				fmt.Println(err)
			}
			err = pubsub.PublishJSON(
				channel,
				string(routing.ExchangePerilTopic),
				routing.ArmyMovesPrefix+"."+username,
				mover,
			)
			if err != nil {
				fmt.Println("Failed to publish move message:", err)
				return
			}
			fmt.Println("Move was published")

		case "status":
			gamestate.CommandStatus()
		case "help":
			gamelogic.PrintClientHelp()
		case "spam":
			number, err := strconv.Atoi(words[1])
			if err != nil {
				fmt.Println("Invalid number:", words[1])
				continue
			}
			fmt.Printf("Spamming %d messages\n", number)
			for i := 0; i < number; i++ {
				fmt.Print("---> ")
				msg := gamelogic.GetMaliciousLog()
				err := pubsub.PublishGob(
					channel,
					routing.ExchangePerilTopic,
					routing.GameLogSlug+"."+username,
					routing.GameLog{
						CurrentTime: time.Now(),
						Message:     msg,
						Username:    username,
					},
				)
				if err != nil {
					fmt.Println("Failed to publish log message:", err)
					return
				}
			}
		case "quit":
			gamelogic.PrintQuit()
			return
		default:
			fmt.Println("Unknown command:", words[0])
		}
	}

}

func handlerPause(gs *gamelogic.GameState) func(routing.PlayingState) pubsub.Acktype {
	return func(ps routing.PlayingState) pubsub.Acktype {
		defer fmt.Print("> ")
		gs.HandlePause(ps)
		return pubsub.Ack
	}
}

func handleArmyMoves(gs *gamelogic.GameState, ch *amqp.Channel) func(gamelogic.ArmyMove) pubsub.Acktype {
	return func(am gamelogic.ArmyMove) pubsub.Acktype {
		defer fmt.Print("> ")
		mover := gs.HandleMove(am)
		switch mover {
		case gamelogic.MoveOutComeSafe:
			return pubsub.Ack
		case gamelogic.MoveOutcomeMakeWar:
			err := pubsub.PublishJSON(
				ch, 
				string(routing.ExchangePerilTopic), 
				routing.WarRecognitionsPrefix + "." + am.Player.Username,
				gamelogic.RecognitionOfWar{
					Attacker: am.Player,
					Defender: gs.GetPlayerSnap(),
				},
			)
			if err != nil {
				fmt.Println("Failed to publish war message:", err)
				return pubsub.NackRequeue
			}
			return pubsub.Ack
		case gamelogic.MoveOutcomeSamePlayer:
			return pubsub.NackDiscard
		default:
			return pubsub.NackDiscard
		}

	}
}

/**
* This function is a handler for the war messages
**/	
func handlerWar(gs *gamelogic.GameState, ch *amqp.Channel) func(gamelogic.RecognitionOfWar) pubsub.Acktype {
	return func(row gamelogic.RecognitionOfWar) pubsub.Acktype {
		defer fmt.Print("> ")
		warOutcome, _, _ := gs.HandleWar(row)

		defer fmt.Print("---> ")
		fmt.Print(warOutcome)
		msg := ""
		switch warOutcome {
		case gamelogic.WarOutcomeNotInvolved:
			return pubsub.NackRequeue
		case gamelogic.WarOutcomeNoUnits:
			return pubsub.NackDiscard
		case gamelogic.WarOutcomeYouWon:
			msg = fmt.Sprintf("%s won a war against %s", row.Attacker.Username, row.Defender.Username)
		case gamelogic.WarOutcomeOpponentWon:
			msg = fmt.Sprintf("%s won a war against %s", row.Defender.Username, row.Attacker.Username)
		case gamelogic.WarOutcomeDraw:
			msg = fmt.Sprintf("A war between %s and %s resulted in a draw", row.Attacker.Username, row.Defender.Username)
		}
		err := pubsub.PublishGob(
			ch, 
			routing.ExchangePerilTopic, 
			routing.GameLogSlug + "." + row.Attacker.Username, 
			routing.GameLog{
				CurrentTime : time.Now(),
				Message     : msg,
				Username    : row.Attacker.Username,
			},
		)

		if err != nil {
			return pubsub.NackRequeue
		}
		return pubsub.Ack
	}
}
