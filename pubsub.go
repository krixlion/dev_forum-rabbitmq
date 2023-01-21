package rabbitmq

import (
	"context"

	amqp "github.com/rabbitmq/amqp091-go"
)

func (mq *RabbitMQ) Publish(ctx context.Context, msg Message) error {
	ctx, span := mq.opts.tracer.Start(ctx, "rabbitmq.Publish")
	defer span.End()

	err := mq.prepareExchange(ctx, msg)
	if err != nil {
		setSpanErr(span, err)
		return err
	}

	err = mq.publish(ctx, msg)
	if err != nil {
		setSpanErr(span, err)
		return err
	}

	return nil
}

// prepareExchange validates a message and declares a RabbitMQ exchange derived from the message.
func (mq *RabbitMQ) prepareExchange(ctx context.Context, msg Message) error {
	ctx, span := mq.opts.tracer.Start(ctx, "rabbitmq.prepareExchange")
	defer span.End()

	ch := mq.askForChannel()
	defer ch.Close()

	if err := ctx.Err(); err != nil {
		setSpanErr(span, err)
		return err
	}

	callSucceded, err := mq.breaker.Allow()
	if err != nil {
		setSpanErr(span, err)
		return err
	}

	err = ch.ExchangeDeclare(
		msg.ExchangeName, // name
		msg.ExchangeType, // type
		true,             // durable
		false,            // auto-deleted
		false,            // internal
		false,            // no-wait
		nil,              // arguments
	)
	if err != nil {
		callSucceded(!isConnectionError(err))
		setSpanErr(span, err)
		return err
	}
	callSucceded(true)

	return nil
}

func (mq *RabbitMQ) publish(ctx context.Context, msg Message) error {
	ctx, span := mq.opts.tracer.Start(ctx, "rabbitmq.publish")
	defer span.End()

	ch := mq.askForChannel()
	defer ch.Close()

	if err := ctx.Err(); err != nil {
		setSpanErr(span, err)
		return err
	}

	callSucceded, err := mq.breaker.Allow()
	if err != nil {
		setSpanErr(span, err)
		return err
	}

	err = ch.PublishWithContext(ctx,
		msg.ExchangeName, // exchange
		msg.RoutingKey,   // routing key
		false,            // mandatory
		false,            // immediate
		amqp.Publishing{
			ContentType: string(msg.ContentType),
			Body:        msg.Body,
			Timestamp:   msg.Timestamp,
		},
	)
	if err != nil {
		callSucceded(!isConnectionError(err))
		setSpanErr(span, err)
		return err
	}
	callSucceded(true)
	return nil
}

func (mq *RabbitMQ) Consume(ctx context.Context, command string, route Route) (<-chan Message, error) {
	messages := make(chan Message)
	ch := mq.askForChannel()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	callSucceded, err := mq.breaker.Allow()
	if err != nil {
		return nil, err
	}

	queue, err := ch.QueueDeclare(
		command, // name
		false,   // durable
		false,   // delete when unused
		false,   // exclusive
		false,   // no-wait
		nil,     // arguments
	)
	if err != nil {
		callSucceded(!isConnectionError(err))
		return nil, err
	}
	callSucceded(true)

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	callSucceded, err = mq.breaker.Allow()
	if err != nil {
		return nil, err
	}

	err = ch.QueueBind(
		queue.Name,         // queue name
		route.RoutingKey,   // routing key
		route.ExchangeName, // exchange
		false,              // Immidiate
		nil,                // Additional args
	)
	if err != nil {
		callSucceded(!isConnectionError(err))
		return nil, err
	}
	callSucceded(true)

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	callSucceded, err = mq.breaker.Allow()
	if err != nil {
		return nil, err
	}

	deliveries, err := ch.Consume(
		queue.Name,      // queue
		mq.consumerName, // consumer
		false,           // auto ack
		false,           // exclusive
		false,           // no local
		false,           // no wait
		nil,             // args
	)
	if err != nil {
		callSucceded(!isConnectionError(err))
		return nil, err
	}
	callSucceded(true)

	go func() {
		for {
			select {
			case delivery := <-deliveries:
				delivery.Ack(false)
				_, span := mq.opts.tracer.Start(ctx, "rabbitmq.Consume")
				defer span.End()

				message := Message{
					Route:       route,
					Body:        delivery.Body,
					ContentType: ContentType(delivery.ContentType),
					Timestamp:   delivery.Timestamp,
				}

				// Non-blocking send to unbuffered channel
				select {
				case messages <- message:
				default:
					continue
				}

			case <-ctx.Done():
				close(messages)
				return
			}
		}
	}()

	return messages, nil
}
