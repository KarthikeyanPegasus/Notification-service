package workflow

import (
	"github.com/spidey/notification-service/internal/config"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.uber.org/zap"
)

func NewClient(cfg *config.Config, log *zap.Logger) (client.Client, error) {
	if cfg.Cadence.Mode == "standalone" {
		log.Warn("temporal client is in standalone mode, workflows will NOT run")
		return nil, nil
	}

	c, err := client.Dial(client.Options{
		HostPort:  cfg.Cadence.HostPort,
		Namespace: cfg.Cadence.Domain,
		Logger:    newTemporalLogger(log),
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

func RegisterWorkflowsAndActivities(w worker.Worker, acts *Activities) {
	w.RegisterWorkflow(NotificationWorkflow)
	w.RegisterWorkflow(OtpNotificationWorkflow)
	w.RegisterWorkflow(BulkNotificationWorkflow)

	w.RegisterActivity(acts.CheckPreferencesActivity)
	w.RegisterActivity(acts.RenderTemplateActivity)
	w.RegisterActivity(acts.PublishToPubSubActivity)
	w.RegisterActivity(acts.LogDeliveryActivity)
	w.RegisterActivity(acts.GenerateOtpActivity)
}

type temporalLogger struct {
	logger *zap.Logger
}

func newTemporalLogger(l *zap.Logger) *temporalLogger {
	return &temporalLogger{logger: l}
}

func (l *temporalLogger) Debug(msg string, keyvals ...interface{}) {
	l.logger.Sugar().Debugw(msg, keyvals...)
}
func (l *temporalLogger) Info(msg string, keyvals ...interface{}) {
	l.logger.Sugar().Infow(msg, keyvals...)
}
func (l *temporalLogger) Warn(msg string, keyvals ...interface{}) {
	l.logger.Sugar().Warnw(msg, keyvals...)
}
func (l *temporalLogger) Error(msg string, keyvals ...interface{}) {
	l.logger.Sugar().Errorw(msg, keyvals...)
}
