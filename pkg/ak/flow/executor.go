package flow

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
	"goauthentik.io/api/v3"
	systemlog "goauthentik.io/platform/pkg/platform/log"
)

var (
	FlowTimingGet = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "authentik_outpost_flow_timing_get_seconds",
		Help: "Duration it took to get a challenge in seconds",
	}, []string{"stage", "flow"})
	FlowTimingPost = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "authentik_outpost_flow_timing_post_seconds",
		Help: "Duration it took to send a challenge in seconds",
	}, []string{"stage", "flow"})
)

type SolverFunction func(*api.ChallengeTypes, api.ApiFlowsExecutorSolveRequest) (api.FlowChallengeResponseRequest, error)

type FlowExecutor struct {
	Params  url.Values
	Answers map[StageComponent]string
	Context context.Context

	solvers map[StageComponent]SolverFunction

	cip       string
	api       *api.APIClient
	flowSlug  string
	log       *log.Entry
	token     string
	session   *http.Cookie
	transport http.RoundTripper

	sp   *sentry.Span
	opts FlowExecutorOptions
	nc   *api.ChallengeTypes
}

type FlowExecutorOptions struct {
	LogFields log.Fields
	Logger    func(msg string, fields map[string]any)
}

func NewFlowExecutor(ctx context.Context, flowSlug string, refConfig *api.Configuration, opts FlowExecutorOptions) (*FlowExecutor, error) {
	rsp := sentry.StartSpan(ctx, "authentik.outposts.flow_executor")
	rsp.Description = flowSlug
	l := systemlog.Get().WithField("flow", flowSlug).WithFields(opts.LogFields)
	if opts.Logger == nil {
		opts.Logger = func(msg string, fields map[string]any) {
			l.WithFields(fields).Debug(msg)
		}
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	fe := &FlowExecutor{
		Params:    url.Values{},
		Answers:   make(map[StageComponent]string),
		Context:   rsp.Context(),
		flowSlug:  flowSlug,
		log:       l,
		sp:        rsp,
		cip:       "",
		transport: http.DefaultTransport,
		opts:      opts,
	}
	fe.solvers = map[StageComponent]SolverFunction{
		StageIdentification:        fe.solveChallenge_Identification,
		StagePassword:              fe.solveChallenge_Password,
		StageAuthenticatorValidate: fe.solveChallenge_AuthenticatorValidate,
		StageUserLogin:             fe.solveChallenge_UserLogin,
	}
	// Create new http client that also sets the correct ip
	config := api.NewConfiguration()
	config.Host = refConfig.Host
	config.Scheme = refConfig.Scheme
	config.HTTPClient = &http.Client{
		Jar:       jar,
		Transport: fe,
	}
	if authz, ok := refConfig.DefaultHeader["Authorization"]; ok {
		fe.token = strings.Split(authz, " ")[1]
	}
	config.AddDefaultHeader(HeaderAuthentikOutpostToken, fe.token)
	fe.api = api.NewAPIClient(config)
	return fe, nil
}

func (fe *FlowExecutor) SetSolver(component StageComponent, solver SolverFunction) {
	fe.solvers[component] = solver
}

func (fe *FlowExecutor) RoundTrip(req *http.Request) (*http.Response, error) {
	res, err := fe.transport.RoundTrip(req)
	if res != nil {
		for _, cookie := range res.Cookies() {
			if cookie.Name == "authentik_session" {
				fe.session = cookie
			}
		}
	}
	return res, err
}

func (fe *FlowExecutor) ApiClient() *api.APIClient {
	return fe.api
}

type ChallengeCommon interface {
	GetComponent() string
	GetResponseErrors() map[string][]api.ErrorDetail
}

func (fe *FlowExecutor) DelegateClientIP(a string) {
	fe.cip = a
	fe.api.GetConfig().AddDefaultHeader(HeaderAuthentikRemoteIP, fe.cip)
}

func (fe *FlowExecutor) getAnswer(stage StageComponent) string {
	if v, o := fe.Answers[stage]; o {
		return v
	}
	return ""
}

func (fe *FlowExecutor) GetSession() *http.Cookie {
	return fe.session
}

func (fe *FlowExecutor) SetSession(s *http.Cookie) {
	fe.session = s
}

// WarmUp Ensure authentik's flow cache is warmed up
func (fe *FlowExecutor) WarmUp() error {
	gcsp := sentry.StartSpan(fe.Context, "authentik.outposts.flow_executor.get_challenge")
	defer gcsp.Finish()
	req := fe.api.FlowsApi.FlowsExecutorGet(gcsp.Context(), fe.flowSlug).Query(fe.Params.Encode())
	_, _, err := req.Execute()
	return err
}

func (fe *FlowExecutor) Start() error {
	initial, err := fe.GetInitialChallenge()
	if err != nil {
		return err
	}
	fe.nc = initial
	return nil
}

func (fe *FlowExecutor) Execute() (bool, error) {
	err := fe.Start()
	if err != nil {
		return false, err
	}
	defer fe.sp.Finish()
	return fe.solver(1)
}

func (fe *FlowExecutor) Challenge() *api.ChallengeTypes {
	return fe.nc
}

func (fe *FlowExecutor) solver(depth int) (bool, error) {
	if depth >= 10 {
		return false, errors.New("exceeded stage recursion depth")
	}
	done, err := fe.SolveFlowChallenge(nil)
	if err != nil {
		return false, err
	}
	if done {
		return true, nil
	}
	return fe.solver(depth + 1)
}

func (fe *FlowExecutor) GetInitialChallenge() (*api.ChallengeTypes, error) {
	// Get challenge
	gcsp := sentry.StartSpan(fe.Context, "authentik.outposts.flow_executor.get_challenge")
	req := fe.api.FlowsApi.FlowsExecutorGet(gcsp.Context(), fe.flowSlug).Query(fe.Params.Encode())
	challenge, _, err := req.Execute()
	if err != nil {
		return nil, err
	}
	i := challenge.GetActualInstance()
	if i == nil {
		return nil, errors.New("response instance was null")
	}
	ch := i.(ChallengeCommon)
	fe.opts.Logger("Got challenge", map[string]interface{}{
		"component": ch.GetComponent(),
	})
	gcsp.SetTag("authentik.flow.component", ch.GetComponent())
	gcsp.Finish()
	FlowTimingGet.With(prometheus.Labels{
		"stage": ch.GetComponent(),
		"flow":  fe.flowSlug,
	}).Observe(float64(gcsp.EndTime.Sub(gcsp.StartTime)) / float64(time.Second))
	return challenge, nil
}

func (fe *FlowExecutor) SolveFlowChallenge(a *api.FlowChallengeResponseRequest) (bool, error) {
	// Resole challenge
	scsp := sentry.StartSpan(fe.Context, "authentik.outposts.flow_executor.solve_challenge")
	responseReq := fe.api.FlowsApi.FlowsExecutorSolve(scsp.Context(), fe.flowSlug).Query(fe.Params.Encode())

	if a != nil {
		responseReq = responseReq.FlowChallengeResponseRequest(*a)
	} else {
		i := fe.nc.GetActualInstance()
		if i == nil {
			return false, errors.New("response request instance was null")
		}
		ch := i.(ChallengeCommon)

		// Check for any validation errors that we might've gotten
		if len(ch.GetResponseErrors()) > 0 {
			for key, errs := range ch.GetResponseErrors() {
				for _, err := range errs {
					return false, fmt.Errorf("flow error %s: %s", key, err.String)
				}
			}
		}
		switch ch.GetComponent() {
		case string(StageAccessDenied):
			return false, nil
		case string(StageRedirect):
			return true, nil
		default:
			solver, ok := fe.solvers[StageComponent(ch.GetComponent())]
			if !ok {
				return false, fmt.Errorf("unsupported challenge type %s", ch.GetComponent())
			}
			rr, err := solver(fe.nc, responseReq)
			if err != nil {
				return false, err
			}
			responseReq = responseReq.FlowChallengeResponseRequest(rr)
		}
	}

	response, _, err := responseReq.Execute()
	if err != nil {
		return false, fmt.Errorf("failed to submit challenge %w", err)
	}
	i := response.GetActualInstance()
	if i == nil {
		return false, errors.New("response instance was null")
	}
	ch := i.(ChallengeCommon)
	fe.opts.Logger("Got response", map[string]interface{}{
		"component": ch.GetComponent(),
	})
	scsp.SetTag("authentik.flow.component", ch.GetComponent())
	scsp.Finish()
	FlowTimingPost.With(prometheus.Labels{
		"stage": ch.GetComponent(),
		"flow":  fe.flowSlug,
	}).Observe(float64(scsp.EndTime.Sub(scsp.StartTime)) / float64(time.Second))
	fe.nc = response
	return false, nil
}
