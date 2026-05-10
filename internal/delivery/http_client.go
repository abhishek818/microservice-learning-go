package delivery

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"microservice-go/internal/model"
)

const (
	HeaderQueueTopic        = "X-Queue-Topic"
	HeaderQueuePartition    = "X-Queue-Partition"
	HeaderQueueOffset       = "X-Queue-Offset"
	HeaderProducerTimestamp = "X-Producer-Timestamp"
)

type Client interface {
	Deliver(ctx context.Context, messageBody []byte, metadata model.DeliveryMetadata) error
}

type HTTPClient struct {
	url        string
	httpClient *http.Client
}

func NewHTTPClient(url string, timeout time.Duration) *HTTPClient {
	return &HTTPClient{
		url: url,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *HTTPClient) Deliver(ctx context.Context, messageBody []byte, metadata model.DeliveryMetadata) error {
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.url,
		bytes.NewReader(messageBody),
	)
	if err != nil {
		return fmt.Errorf("failed to create http request: %w", err)
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(HeaderQueueTopic, metadata.Topic)
	request.Header.Set(HeaderQueuePartition, strconv.Itoa(metadata.Partition))
	request.Header.Set(HeaderQueueOffset, strconv.FormatInt(metadata.Offset, 10))
	if !metadata.ProducerTimestamp.IsZero() {
		request.Header.Set(HeaderProducerTimestamp, metadata.ProducerTimestamp.UTC().Format(time.RFC3339Nano))
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("http request failed: %w", err)
	}
	defer response.Body.Close()

	responseBody, _ := io.ReadAll(response.Body)

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf(
			"microservice-2 returned non-success status: status_code=%d response=%s",
			response.StatusCode,
			strings.TrimSpace(string(responseBody)),
		)
	}

	return nil
}

func MetadataFromHeaders(headers http.Header) (model.DeliveryMetadata, error) {
	metadata := model.DeliveryMetadata{
		Topic: strings.TrimSpace(headers.Get(HeaderQueueTopic)),
	}

	partitionValue := strings.TrimSpace(headers.Get(HeaderQueuePartition))
	if partitionValue != "" {
		partition, err := strconv.Atoi(partitionValue)
		if err != nil {
			return model.DeliveryMetadata{}, fmt.Errorf("invalid %s header: %w", HeaderQueuePartition, err)
		}
		metadata.Partition = partition
	}

	offsetValue := strings.TrimSpace(headers.Get(HeaderQueueOffset))
	if offsetValue != "" {
		offset, err := strconv.ParseInt(offsetValue, 10, 64)
		if err != nil {
			return model.DeliveryMetadata{}, fmt.Errorf("invalid %s header: %w", HeaderQueueOffset, err)
		}
		metadata.Offset = offset
	}

	producerTimestampValue := strings.TrimSpace(headers.Get(HeaderProducerTimestamp))
	if producerTimestampValue != "" {
		producerTimestamp, err := time.Parse(time.RFC3339Nano, producerTimestampValue)
		if err != nil {
			return model.DeliveryMetadata{}, fmt.Errorf("invalid %s header: %w", HeaderProducerTimestamp, err)
		}
		metadata.ProducerTimestamp = producerTimestamp
	}

	return metadata, nil
}
