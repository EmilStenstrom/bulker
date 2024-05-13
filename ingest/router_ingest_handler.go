package main

import (
	"compress/gzip"
	"fmt"
	kafka2 "github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/gin-gonic/gin"
	"github.com/jitsucom/bulker/eventslog"
	"github.com/jitsucom/bulker/jitsubase/appbase"
	"github.com/jitsucom/bulker/jitsubase/jsoniter"
	"github.com/jitsucom/bulker/jitsubase/jsonorder"
	"github.com/jitsucom/bulker/jitsubase/types"
	"github.com/jitsucom/bulker/jitsubase/utils"
	"github.com/jitsucom/bulker/jitsubase/uuid"
	"io"
	"net/http"
	"strings"
)

func (r *Router) IngestHandler(c *gin.Context) {
	domain := ""
	// TODO: use workspaceId as default for all stream identification errors
	var eventsLogId string
	var ingestType IngestType
	var rError *appbase.RouterError
	var body []byte
	var ingestMessageBytes []byte
	var asyncDestinations []string
	var tagsDestinations []string

	defer func() {
		if len(ingestMessageBytes) == 0 {
			ingestMessageBytes = body
		}
		if len(ingestMessageBytes) > 0 {
			_ = r.backupsLogger.Log(utils.DefaultString(eventsLogId, "UNKNOWN"), ingestMessageBytes)
		}
		if rError != nil && rError.ErrorType != ErrNoDst {
			obj := map[string]any{"body": string(ingestMessageBytes), "error": rError.PublicError.Error(), "status": utils.Ternary(rError.ErrorType == ErrThrottledType, "SKIPPED", "FAILED")}
			r.eventsLogService.PostAsync(&eventslog.ActorEvent{EventType: eventslog.EventTypeIncoming, Level: eventslog.LevelError, ActorId: eventsLogId, Event: obj})
			IngestHandlerRequests(domain, utils.Ternary(rError.ErrorType == ErrThrottledType, "throttled", "error"), rError.ErrorType).Inc()
			_ = r.producer.ProduceAsync(r.config.KafkaDestinationsDeadLetterTopicName, uuid.New(), ingestMessageBytes, map[string]string{"error": rError.Error.Error()}, kafka2.PartitionAny)
		} else {
			obj := map[string]any{"body": string(ingestMessageBytes), "asyncDestinations": asyncDestinations, "tags": tagsDestinations}
			if len(asyncDestinations) > 0 || len(tagsDestinations) > 0 {
				obj["status"] = "SUCCESS"
			} else {
				obj["status"] = "SKIPPED"
				obj["error"] = "no destinations found for stream"
			}
			r.eventsLogService.PostAsync(&eventslog.ActorEvent{EventType: eventslog.EventTypeIncoming, Level: eventslog.LevelInfo, ActorId: eventsLogId, Event: obj})
			IngestHandlerRequests(domain, "success", "").Inc()
		}
	}()
	defer func() {
		if rerr := recover(); rerr != nil {
			rError = r.ResponseError(c, http.StatusInternalServerError, "panic", true, fmt.Errorf("%v", rerr), true)
		}
	}()
	c.Set(appbase.ContextLoggerName, "ingest")
	if !strings.HasSuffix(c.ContentType(), "application/json") && !strings.HasSuffix(c.ContentType(), "text/plain") {
		rError = r.ResponseError(c, http.StatusBadRequest, "invalid content type", false, fmt.Errorf("%s. Expected: application/json", c.ContentType()), true)
		return
	}
	if c.FullPath() == "/api/s/s2s/:tp" {
		ingestType = IngestTypeS2S
	} else {
		ingestType = IngestTypeBrowser
	}
	tp := c.Param("tp")
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		err = fmt.Errorf("Client Ip: %s: %v", utils.NvlString(c.GetHeader("X-Real-Ip"), c.GetHeader("X-Forwarded-For"), c.ClientIP()), err)
		rError = r.ResponseError(c, http.StatusOK, "error reading HTTP body", false, err, true)
		return
	}
	var message types.Json
	err = jsonorder.Unmarshal(body, &message)
	if err != nil {
		rError = r.ResponseError(c, http.StatusOK, "error parsing message", false, fmt.Errorf("%v: %s", err, string(body)), true)
		return
	}
	messageId := message.GetS("messageId")
	if messageId == "" {
		messageId = uuid.New()
	} else {
		messageId = utils.ShortenString(messageIdUnsupportedChars.ReplaceAllString(messageId, "_"), 64)
	}
	c.Set(appbase.ContextMessageId, messageId)
	//func() string { wk, _ := message["writeKey"].(string); return wk }
	loc, err := r.getDataLocator(c, ingestType, nil)
	if err != nil {
		rError = r.ResponseError(c, http.StatusOK, "error processing message", false, fmt.Errorf("%v: %s", err, string(body)), true)
		return
	}
	domain = utils.DefaultString(loc.Slug, loc.Domain)
	c.Set(appbase.ContextDomain, domain)

	stream := r.getStream(&loc)
	if stream == nil {
		rError = r.ResponseError(c, http.StatusOK, "stream not found", false, fmt.Errorf("for: %+v", loc), true)
		return
	}
	eventsLogId = stream.Stream.Id
	ingestMessage, ingestMessageBytes, err := r.buildIngestMessage(c, messageId, message, nil, tp, loc, stream)
	if err != nil {
		rError = r.ResponseError(c, http.StatusOK, "event error", false, err, true)
		return
	}
	if len(stream.AsynchronousDestinations) == 0 && len(stream.SynchronousDestinations) == 0 {
		rError = r.ResponseError(c, http.StatusOK, ErrNoDst, false, fmt.Errorf(stream.Stream.Id), true)
		return
	}
	asyncDestinations, tagsDestinations, rError = r.sendToRotor(c, ingestMessageBytes, stream, true)
	if rError != nil {
		return
	}
	if len(tagsDestinations) == 0 {
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}
	resp := r.processSyncDestination(ingestMessage, stream, ingestMessageBytes)
	if resp != nil {
		if r.ShouldCompress(c.Request) {
			c.Header("Content-Encoding", "gzip")
			c.Header("Content-Type", "application/json")
			c.Header("Vary", "Accept-Encoding")
			gz := gzip.NewWriter(c.Writer)
			_ = jsoniter.NewEncoder(gz).Encode(resp)
			_ = gz.Close()
		} else {
			c.JSON(http.StatusOK, resp)
		}
	} else {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}
