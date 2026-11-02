package dispatchers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"sync"

	"github.com/chetansachan/socialflow/notification-service/internal/models"
)

// EmailProvider is implemented by the log provider and SMTP. SendGrid/SES can
// be added as another implementation without touching the worker pool.
type EmailProvider interface {
	Send(ctx context.Context, message models.EmailMessage) error
}

// EmailDispatcher is a bounded worker pool for transactional email. Like push,
// it keeps external provider latency off the event-consumer goroutine.
type EmailDispatcher struct {
	provider EmailProvider
	queue    chan models.EmailMessage
	workers  int
	logger   *log.Logger
	wg       sync.WaitGroup
}

func NewEmailDispatcher(provider EmailProvider, workers, queueSize int, logger *log.Logger) *EmailDispatcher {
	if workers <= 0 {
		workers = 2
	}
	if queueSize <= 0 {
		queueSize = 256
	}
	if logger == nil {
		logger = log.Default()
	}
	return &EmailDispatcher{
		provider: provider,
		queue:    make(chan models.EmailMessage, queueSize),
		workers:  workers,
		logger:   logger,
	}
}

func (d *EmailDispatcher) Start(ctx context.Context) {
	for i := 0; i < d.workers; i++ {
		d.wg.Add(1)
		go d.worker(ctx)
	}
}

func (d *EmailDispatcher) Wait() {
	d.wg.Wait()
}

func (d *EmailDispatcher) Enqueue(ctx context.Context, message models.EmailMessage) error {
	if d == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case d.queue <- message:
		return nil
	default:
		return ErrQueueFull
	}
}

func (d *EmailDispatcher) worker(ctx context.Context) {
	defer d.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case message := <-d.queue:
			if err := retry(ctx, 2, func() error {
				return d.provider.Send(ctx, message)
			}); err != nil {
				d.logger.Printf("email: delivery failed for user %s: %v", message.RecipientID, err)
			}
		}
	}
}

// LogEmailProvider is the local-development default.
type LogEmailProvider struct {
	logger *log.Logger
}

func NewLogEmailProvider(logger *log.Logger) *LogEmailProvider {
	if logger == nil {
		logger = log.Default()
	}
	return &LogEmailProvider{logger: logger}
}

func (p *LogEmailProvider) Send(ctx context.Context, message models.EmailMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.logger.Printf("email[log] -> to=%s subject=%q body=%q", message.RecipientEmail, message.Subject, truncate(message.Body, 200))
	return nil
}

// SMTPEmailProvider works with SendGrid, Amazon SES, Postmark, and any
// standards-compliant SMTP relay. STARTTLS is negotiated automatically by
// net/smtp when the server advertises it.
type SMTPEmailProvider struct {
	host     string
	port     int
	username string
	password string
	from     string
}

func NewSMTPEmailProvider(host string, port int, username, password, from string) (*SMTPEmailProvider, error) {
	if strings.TrimSpace(host) == "" {
		return nil, fmt.Errorf("SMTP host is required")
	}
	if port <= 0 {
		port = 587
	}
	if strings.TrimSpace(from) == "" {
		return nil, fmt.Errorf("email from address is required")
	}
	return &SMTPEmailProvider{
		host:     host,
		port:     port,
		username: username,
		password: password,
		from:     from,
	}, nil
}

func (p *SMTPEmailProvider) Send(ctx context.Context, message models.EmailMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := message.Validate(); err != nil {
		return err
	}

	subject := sanitizeHeader(message.Subject)
	body := strings.ReplaceAll(message.Body, "\n", "\r\n")
	content := strings.Join([]string{
		"From: " + p.from,
		"To: " + message.RecipientEmail,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")

	address := net.JoinHostPort(p.host, strconv.Itoa(p.port))
	var auth smtp.Auth
	if p.username != "" {
		auth = smtp.PlainAuth("", p.username, p.password, p.host)
	}

	if err := smtp.SendMail(address, auth, p.from, []string{message.RecipientEmail}, []byte(content)); err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		return fmt.Errorf("failed to send SMTP email: %w", err)
	}
	return nil
}

func sanitizeHeader(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	return strings.ReplaceAll(value, "\n", " ")
}
