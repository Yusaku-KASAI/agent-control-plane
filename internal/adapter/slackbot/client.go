package slackbot

import (
	"net/http"
	"time"

	"github.com/slack-go/slack"
)

// webRetry is how calls to Slack's Web API ride out a network that drops or hangs for a while
// (docs/adr/app/0063).
type webRetry struct {
	// timeout bounds each attempt: without it, a call whose answer never comes waits forever.
	timeout time.Duration
	// retries is how many times a call is tried again after its first attempt.
	retries int
	// initial is the wait before the first retry, doubled for each next one up to max.
	initial, max time.Duration
}

// defaultWebRetry waits about a minute in all between the attempts of a call.
var defaultWebRetry = webRetry{timeout: 30 * time.Second, retries: 6, initial: time.Second, max: 30 * time.Second}

// newWebClient returns a Web API client that retries calls that got no answer, a server error or a
// rate limit. Slack's own refusals, such as an invalid message, are not retried.
func newWebClient(token, appToken string, r webRetry, opts ...slack.Option) *slack.Client {
	cfg := slack.DefaultRetryConfig()
	cfg.MaxRetries = r.retries
	cfg.Handlers = []slack.RetryHandler{unanswered(r), slack.NewRateLimitErrorRetryHandler(cfg)}
	// The retry wraps the HTTP client set before it, so the timeout bounds each attempt.
	opts = append([]slack.Option{
		slack.OptionAppLevelToken(appToken),
		slack.OptionHTTPClient(&http.Client{Timeout: r.timeout}),
		slack.OptionRetryConfig(cfg),
	}, opts...)
	return slack.New(token, opts...)
}

// unanswered retries a call that got no answer or a server error. slack-go's handler for connection
// errors knows only a few messages, missing timeouts and unreachable networks.
type unanswered webRetry

func (u unanswered) ShouldRetry(state *slack.RetryState, req *http.Request, resp *http.Response, err error) (bool, time.Duration) {
	// slack-go asks only while attempts remain and the request's body can be sent again.
	if failed := (resp == nil && err != nil) || (resp != nil && resp.StatusCode >= http.StatusInternalServerError); !failed {
		return false, 0
	}
	return true, min(u.initial<<state.Attempt, u.max)
}
