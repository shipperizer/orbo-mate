package logger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	Log              *zap.Logger
	Sugar            *zap.SugaredLogger
	once             sync.Once
	activeLokiWriter *lokiWriter
)

func parseLogLevel(levelStr string) zapcore.Level {
	if levelStr == "" {
		levelStr = os.Getenv("LOG_LEVEL")
	}
	switch levelStr {
	case "debug", "DEBUG":
		return zapcore.DebugLevel
	case "info", "INFO", "":
		return zapcore.InfoLevel
	case "warn", "WARN", "warning", "WARNING":
		return zapcore.WarnLevel
	case "error", "ERROR":
		return zapcore.ErrorLevel
	case "fatal", "FATAL":
		return zapcore.FatalLevel
	default:
		return zapcore.InfoLevel
	}
}

func init() {
	level := parseLogLevel("")
	// Initialize with a default production logger so it's never nil
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	jsonEncoder := zapcore.NewJSONEncoder(encoderConfig)
	stdoutSyncer := zapcore.Lock(os.Stdout)
	core := zapcore.NewCore(jsonEncoder, stdoutSyncer, level)
	Log = zap.New(core)
	Sugar = Log.Sugar()
}

// Init initializes the global logger package with stdout JSON logging and optional Loki exporter.
func Init(logLevel string) {
	once.Do(func() {
		level := parseLogLevel(logLevel)
		encoderConfig := zap.NewProductionEncoderConfig()
		encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		jsonEncoder := zapcore.NewJSONEncoder(encoderConfig)

		stdoutSyncer := zapcore.Lock(os.Stdout)

		var cores []zapcore.Core
		cores = append(cores, zapcore.NewCore(jsonEncoder, stdoutSyncer, level))

		// Check for Loki endpoint
		lokiURL := os.Getenv("LOKI_ENDPOINT")
		if lokiURL == "" {
			lokiURL = os.Getenv("LOKI_URL")
		}

		if lokiURL != "" {
			lokiWriter := newLokiWriter(lokiURL)
			activeLokiWriter = lokiWriter
			go lokiWriter.start()
			cores = append(cores, zapcore.NewCore(jsonEncoder, lokiWriter, level))
		}

		combinedCore := zapcore.NewTee(cores...)
		Log = zap.New(combinedCore, zap.AddCaller(), zap.AddCallerSkip(1))
		Sugar = Log.Sugar()
	})
}

// Shutdown flushes buffered logs and stops any background workers.
func Shutdown() {
	if activeLokiWriter != nil {
		activeLokiWriter.Stop()
	}
	if Log != nil {
		_ = Log.Sync()
	}
}

// Log functions that delegate to the global logger.
func Info(msg string, fields ...zap.Field) {
	Log.Info(msg, fields...)
}

func Infof(template string, args ...interface{}) {
	Sugar.Infof(template, args...)
}

func Error(msg string, fields ...zap.Field) {
	Log.Error(msg, fields...)
}

func Errorf(template string, args ...interface{}) {
	Sugar.Errorf(template, args...)
}

func Warn(msg string, fields ...zap.Field) {
	Log.Warn(msg, fields...)
}

func Warnf(template string, args ...interface{}) {
	Sugar.Warnf(template, args...)
}

func Fatal(msg string, fields ...zap.Field) {
	Log.Fatal(msg, fields...)
}

func Fatalf(template string, args ...interface{}) {
	Sugar.Fatalf(template, args...)
}

func Debug(msg string, fields ...zap.Field) {
	Log.Debug(msg, fields...)
}

func Debugf(template string, args ...interface{}) {
	Sugar.Debugf(template, args...)
}

type lokiWriter struct {
	url      string
	logChan  chan string
	client   *http.Client
	stopChan chan struct{}
	wg       sync.WaitGroup
}

func newLokiWriter(url string) *lokiWriter {
	return &lokiWriter{
		url:      url,
		logChan:  make(chan string, 1000),
		client:   &http.Client{Timeout: 5 * time.Second},
		stopChan: make(chan struct{}),
	}
}

func (w *lokiWriter) Write(p []byte) (n int, err error) {
	line := string(p)
	select {
	case w.logChan <- line:
	default:
		// Drop logs if queue is full to prevent blocking
	}
	return len(p), nil
}

func (w *lokiWriter) Sync() error {
	return nil
}

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][2]string       `json:"values"`
}

type lokiPushRequest struct {
	Streams []lokiStream `json:"streams"`
}

func (w *lokiWriter) start() {
	w.wg.Add(1)
	defer w.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var batch []string
	maxBatchSize := 100

	sendBatch := func() {
		if len(batch) == 0 {
			return
		}

		streams := []lokiStream{
			{
				Stream: map[string]string{
					"app": "orbo-mate",
				},
				Values: make([][2]string, 0, len(batch)),
			},
		}

		baseTime := time.Now().UnixNano()
		for idx, logLine := range batch {
			// Offset by index to keep timestamps unique and in order within the batch
			nano := fmt.Sprintf("%d", baseTime+int64(idx))
			streams[0].Values = append(streams[0].Values, [2]string{nano, logLine})
		}

		payload := lokiPushRequest{Streams: streams}
		bodyBytes, err := json.Marshal(payload)
		if err != nil {
			return
		}

		req, err := http.NewRequest("POST", w.url, bytes.NewBuffer(bodyBytes))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := w.client.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()

		batch = nil
	}

	for {
		select {
		case logLine := <-w.logChan:
			batch = append(batch, logLine)
			if len(batch) >= maxBatchSize {
				sendBatch()
			}
		case <-ticker.C:
			sendBatch()
		case <-w.stopChan:
			// Drain remaining logs
			for {
				select {
				case logLine := <-w.logChan:
					batch = append(batch, logLine)
					if len(batch) >= maxBatchSize {
						sendBatch()
					}
				default:
					sendBatch()
					return
				}
			}
		}
	}
}

func (w *lokiWriter) Stop() {
	close(w.stopChan)
	w.wg.Wait()
}
