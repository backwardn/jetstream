// Copyright 2020 The NATS Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package jsch

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// DefaultConsumer is the configuration that will be used to create new Consumers in NewConsumer
var DefaultConsumer = server.ObservableConfig{
	DeliverAll:   true,
	AckPolicy:    server.AckExplicit,
	AckWait:      30 * time.Second,
	ReplayPolicy: server.ReplayInstant,
}

// SampledDefaultConsumer is the configuration that will be used to create new Consumers in NewConsumer
var SampledDefaultConsumer = server.ObservableConfig{
	DeliverAll:      true,
	AckPolicy:       server.AckExplicit,
	AckWait:         30 * time.Second,
	ReplayPolicy:    server.ReplayInstant,
	SampleFrequency: "100%",
}

// ConsumerOptions configures consumers
type ConsumerOption func(o *server.ObservableConfig) error

// Consumer represents a JetStream consumer
type Consumer struct {
	name   string
	stream string
	cfg    server.ObservableConfig
}

// NewConsumerFromTemplate creates a new consumer based on a template config that gets modified by opts
func NewConsumerFromTemplate(stream string, template server.ObservableConfig, opts ...ConsumerOption) (consumer *Consumer, err error) {
	cfg, err := NewConsumerConfiguration(template, opts...)
	if err != nil {
		return nil, err
	}

	req := server.CreateObservableRequest{
		MsgSet: stream,
		Config: cfg,
	}

	jreq, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	response, err := nc.Request(server.JetStreamCreateObservable, jreq, Timeout)
	if err != nil {
		return nil, err
	}

	if !strings.HasPrefix(string(response.Data), server.OK) {
		return nil, fmt.Errorf("%s", string(response.Data))
	}

	if cfg.Durable != "" {
		return LoadConsumer(stream, cfg.Durable)
	}

	return nil, nil
}

// NewConsumer creates a consumer based on DefaultConsumer modified by opts
func NewConsumer(stream string, opts ...ConsumerOption) (consumer *Consumer, err error) {
	return NewConsumerFromTemplate(stream, DefaultConsumer, opts...)
}

// LoadOrNewConsumer loads a consumer by name if known else creates a new one with these properties
func LoadOrNewConsumer(stream string, name string, opts ...ConsumerOption) (consumer *Consumer, err error) {
	return LoadOrNewConsumerFromTemplate(stream, name, DefaultConsumer, opts...)
}

// LoadOrNewConsumerFromTemplate loads a consumer by name if known else creates a new one with these properties based on template
func LoadOrNewConsumerFromTemplate(stream string, name string, template server.ObservableConfig, opts ...ConsumerOption) (consumer *Consumer, err error) {
	c, err := LoadConsumer(stream, name)
	if c == nil || err != nil {
		return NewConsumerFromTemplate(stream, template, opts...)
	}

	return c, err
}

// LoadConsumer loads a consumer by name
func LoadConsumer(stream string, name string) (consumer *Consumer, err error) {
	consumer = &Consumer{
		name:   name,
		stream: stream,
	}

	err = loadConfigForConsumer(consumer)
	if err != nil {
		return nil, err
	}

	return consumer, nil
}

// NewConsmerConfiguration generates a new configuration based on template modified by opts
func NewConsumerConfiguration(template server.ObservableConfig, opts ...ConsumerOption) (server.ObservableConfig, error) {
	for _, o := range opts {
		err := o(&template)
		if err != nil {
			return template, err
		}
	}

	return template, nil
}

func loadConfigForConsumer(consumer *Consumer) (err error) {
	info, err := loadConsumerInfo(consumer.stream, consumer.name)
	if err != nil {
		return err
	}

	consumer.cfg = info.Config

	return nil
}

func loadConsumerInfo(s string, c string) (info server.ObservableInfo, err error) {
	response, err := nc.Request(server.JetStreamObservableInfo, []byte(s+" "+c), Timeout)
	if err != nil {
		return info, err
	}

	if IsErrorResponse(response) {
		return info, fmt.Errorf("%s", string(response.Data))
	}

	info = server.ObservableInfo{}
	err = json.Unmarshal(response.Data, &info)
	if err != nil {
		return info, err
	}

	return info, nil
}

func DeliverySubject(s string) ConsumerOption {
	return func(o *server.ObservableConfig) error {
		o.Delivery = s
		return nil
	}
}

func DurableName(s string) ConsumerOption {
	return func(o *server.ObservableConfig) error {
		o.Durable = s
		return nil
	}
}

func StartAtSequence(s uint64) ConsumerOption {
	return func(o *server.ObservableConfig) error {
		o.MsgSetSeq = s
		return nil
	}
}

func StartAtTime(t time.Time) ConsumerOption {
	return func(o *server.ObservableConfig) error {
		o.StartTime = t
		return nil
	}
}

func DeliverAllAvailable() ConsumerOption {
	return func(o *server.ObservableConfig) error {
		o.DeliverAll = true
		return nil
	}
}

func StartWithLastReceived() ConsumerOption {
	return func(o *server.ObservableConfig) error {
		o.DeliverLast = true
		return nil
	}
}

func StartAtTimeDelta(d time.Duration) ConsumerOption {
	return func(o *server.ObservableConfig) error {
		o.StartTime = time.Now().Add(-1 * d)
		return nil
	}
}

func AcknowledgeNone() ConsumerOption {
	return func(o *server.ObservableConfig) error {
		o.AckPolicy = server.AckNone
		return nil
	}
}

func AcknowledgeAll() ConsumerOption {
	return func(o *server.ObservableConfig) error {
		o.AckPolicy = server.AckAll
		return nil
	}
}

func AcknowledgeExplicit() ConsumerOption {
	return func(o *server.ObservableConfig) error {
		o.AckPolicy = server.AckExplicit
		return nil
	}
}

func AckWait(t time.Duration) ConsumerOption {
	return func(o *server.ObservableConfig) error {
		o.AckWait = t
		return nil
	}
}

func MaxDeliveryAttempts(n int) ConsumerOption {
	return func(o *server.ObservableConfig) error {
		if n == 0 {
			return fmt.Errorf("configuration would prevent all deliveries")
		}
		o.MaxDeliver = n
		return nil
	}
}

func FilterStreamBySubject(s string) ConsumerOption {
	return func(o *server.ObservableConfig) error {
		o.FilterSubject = s
		return nil
	}
}

func ReplayInstantly() ConsumerOption {
	return func(o *server.ObservableConfig) error {
		o.ReplayPolicy = server.ReplayInstant
		return nil
	}
}

func ReplayAsReceived() ConsumerOption {
	return func(o *server.ObservableConfig) error {
		o.ReplayPolicy = server.ReplayOriginal
		return nil
	}
}

func SamplePercent(i int) ConsumerOption {
	return func(o *server.ObservableConfig) error {
		if i < 0 || i > 100 {
			return fmt.Errorf("sample percent must be 0-100")
		}

		if i == 0 {
			o.SampleFrequency = ""
			return nil
		}

		o.SampleFrequency = fmt.Sprintf("%d%%", i)
		return nil
	}
}

// Reset reloads the Consumer configuration from the JetStream server
func (c *Consumer) Reset() error {
	return loadConfigForConsumer(c)
}

// NextSubject returns the subject used to retrieve the next message for pull-based Consumers, empty when not a pull-base consumer
func (c *Consumer) NextSubject() string {
	if !c.IsPullMode() {
		return ""
	}

	return server.JetStreamRequestNextPre + "." + c.stream + "." + c.name
}

// SampleSubject is the subject used to publish ack samples to
func (c *Consumer) SampleSubject() string {
	if c.SampleFrequency() == "" {
		return ""
	}

	return server.JetStreamObservableAckSamplePre + "." + c.StreamName() + "." + c.name
}

// Subscribe see nats.Subscribe
func (c *Consumer) Subscribe(h func(*nats.Msg)) (sub *nats.Subscription, err error) {
	if !c.IsPushMode() {
		return nil, fmt.Errorf("consumer %s > %s is not push-based", c.stream, c.name)
	}

	return nc.Subscribe(c.DeliverySubject(), h)
}

// ChanSubscribe see nats.ChangSubscribe
func (c *Consumer) ChanSubscribe(ch chan *nats.Msg) (sub *nats.Subscription, err error) {
	if !c.IsPushMode() {
		return nil, fmt.Errorf("consumer %s > %s is not push-based", c.stream, c.name)
	}

	return nc.ChanSubscribe(c.DeliverySubject(), ch)
}

// ChanQueueSubscribe see nats.ChanQueueSubscribe
func (c *Consumer) ChanQueueSubscribe(group string, ch chan *nats.Msg) (sub *nats.Subscription, err error) {
	if !c.IsPushMode() {
		return nil, fmt.Errorf("consumer %s > %s is not push-based", c.stream, c.name)
	}

	return nc.ChanQueueSubscribe(c.DeliverySubject(), group, ch)
}

// SubscribeSync see nats.SubscribeSync
func (c *Consumer) SubscribeSync() (sub *nats.Subscription, err error) {
	if !c.IsPushMode() {
		return nil, fmt.Errorf("consumer %s > %s is not push-based", c.stream, c.name)
	}

	return nc.SubscribeSync(c.DeliverySubject())
}

// QueueSubscribe see nats.QueueSubscribe
func (c *Consumer) QueueSubscribe(queue string, h func(*nats.Msg)) (sub *nats.Subscription, err error) {
	if !c.IsPushMode() {
		return nil, fmt.Errorf("consumer %s > %s is not push-based", c.stream, c.name)
	}

	return nc.QueueSubscribe(c.DeliverySubject(), queue, h)
}

// QueueSubscribeSync see nats.QueueSubscribeSync
func (c *Consumer) QueueSubscribeSync(queue string) (sub *nats.Subscription, err error) {
	if !c.IsPushMode() {
		return nil, fmt.Errorf("consumer %s > %s is not push-based", c.stream, c.name)
	}

	return nc.QueueSubscribeSync(c.DeliverySubject(), queue)
}

// QueueSubscribeSyncWithChan see nats.QueueSubscribeSyncWithChan
func (c *Consumer) QueueSubscribeSyncWithChan(queue string, ch chan *nats.Msg) (sub *nats.Subscription, err error) {
	if !c.IsPushMode() {
		return nil, fmt.Errorf("consumer %s > %s is not push-based", c.stream, c.name)
	}

	return nc.QueueSubscribeSyncWithChan(c.DeliverySubject(), queue, ch)
}

// NextMsgs retrieves the next n messages
func (c *Consumer) NextMsgs(n int) (m *nats.Msg, err error) {
	if !c.IsPullMode() {
		return nil, fmt.Errorf("consumer %s > %s is not pull-based", c.stream, c.name)
	}

	return nc.Request(c.NextSubject(), []byte(strconv.Itoa(n)), Timeout)
}

// NextMsg retrieves the next message
func (c *Consumer) NextMsg() (m *nats.Msg, err error) {
	return c.NextMsgs(1)
}

// State returns the Consumer state
func (c *Consumer) State() (stats server.ObservableState, err error) {
	info, err := loadConsumerInfo(c.stream, c.name)
	if err != nil {
		return server.ObservableState{}, err
	}

	return info.State, nil
}

// Configuration is the Consumer configuration
func (c *Consumer) Configuration() (config server.ObservableConfig) {
	return c.cfg
}

// Delete deletes the Consumer, after this the Consumer object should be disposed
func (c *Consumer) Delete() (err error) {
	response, err := nc.Request(server.JetStreamDeleteObservable, []byte(c.StreamName()+" "+c.Name()), Timeout)
	if err != nil {
		return err
	}

	if IsErrorResponse(response) {
		return fmt.Errorf("%s", string(response.Data))
	}

	if IsOKResponse(response) {
		return nil
	}

	return fmt.Errorf("unknown response while removing consumer %s: %q", c.Name(), response.Data)
}

func (c *Consumer) Name() string                      { return c.name }
func (c *Consumer) IsSampled() bool                   { return c.SampleSubject() != "" }
func (c *Consumer) IsPullMode() bool                  { return c.cfg.Delivery == "" }
func (c *Consumer) IsPushMode() bool                  { return !c.IsPullMode() }
func (c *Consumer) IsDurable() bool                   { return c.cfg.Durable != "" }
func (c *Consumer) StreamName() string                { return c.stream }
func (c *Consumer) DeliverySubject() string           { return c.cfg.Delivery }
func (c *Consumer) DurableName() string               { return c.cfg.Durable }
func (c *Consumer) StreamSequence() uint64            { return c.cfg.MsgSetSeq }
func (c *Consumer) StartTime() time.Time              { return c.cfg.StartTime }
func (c *Consumer) DeliverAll() bool                  { return c.cfg.DeliverAll }
func (c *Consumer) DeliverLast() bool                 { return c.cfg.DeliverLast }
func (c *Consumer) AckPolicy() server.AckPolicy       { return c.cfg.AckPolicy }
func (c *Consumer) AckWait() time.Duration            { return c.cfg.AckWait }
func (c *Consumer) MaxDeliver() int                   { return c.cfg.MaxDeliver }
func (c *Consumer) FilterSubject() string             { return c.cfg.FilterSubject }
func (c *Consumer) ReplayPolicy() server.ReplayPolicy { return c.cfg.ReplayPolicy }
func (c *Consumer) SampleFrequency() string           { return c.cfg.SampleFrequency }