package test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/restmapper"
	clienttesting "k8s.io/client-go/testing"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// ControllerTestCase represents a test case for a controller.
type ControllerTestCase struct {
	// Name is the name of the test
	Name string
	// ConfigureGlobals is a function that configures global environment variables
	// required for the test.
	ConfigureGlobals func()
	// AddControllers is a list of functions which add a controller to the controller manager.
	// This has the same signature as the Add() method created by the operator-sdk.
	AddControllers []AddControllerFunc
	// Known resource types, used to seed RESTMapper
	GroupResources []*restmapper.APIGroupResources
	// Resources are existing resources that should be seeded into the ObjectTracker
	// used by the simulator.
	Resources []runtime.Object
	// Events are events that should be fed through the controller manager.
	Events []ControllerTestEvent
}

// AddControllerFunc represents a function that adds a controller to the controller manager simulator.
// This is the same signature as the Add() method generated by the operator-sdk when creating
// a new controller.
type AddControllerFunc func(mgr manager.Manager) error

// ControllerTestEvent represents an event that is sent through the controller manager,
// including all necessary reactions and verification logic.
type ControllerTestEvent struct {
	// Name of test event, e.g. bootstrap-clean-install-no-errors.  This will seed the test name for the event.
	Name string
	// Execute is a function that triggers some event, e.g. mgr.GetClient().Create(someTestResource).
	Execute GenerateEventFunc
	// Verifier is an ActionVerifier that verifies a specific response from the system, e.g.
	// verify that a status update occurred.  ActionVerifiers (list) can be used to ensure
	// a specific, ordered sequence of actions has been triggered by the controller under test.
	Verifier ActionVerifier
	// Assertions to be made regarding the processing of the event, e.g. 15 CRD resources were created.
	Assertions []ActionAssertion
	// Reactors are any custom reactions that should intercept actions generated by
	// the controller under test, e.g. returning a NotFoundError for a particular client.Get()
	// invocation.
	Reactors []clienttesting.Reactor
	// Timeout is the maximum amount of time to wait for the Verifier to be triggered.
	Timeout time.Duration
}

// GenerateEventFunc is a function which triggers some test action.
type GenerateEventFunc func(mgr *FakeManager, tracker *EnhancedTracker) error

// ActionVerifier is a specialized Reactor that is used to verify an action (event)
// generated by the controller, e.g. verify that a status update occurred on a
// particular object.
type ActionVerifier interface {
	clienttesting.Reactor
	// Wait for verification.  This call will block until the verification occurs
	// or the timeout period has elapsed.  Returns true if the call timed out.
	Wait(timeout time.Duration) (timedout bool)
	// HasFired returns true if this validator has fired
	HasFired() bool
	// InjectTestRunner injects the test runner into the verifier.
	InjectTestRunner(t *testing.T)
}

// ActionVerifierFunc is a function that performs verification logic against some action.
// An error should be returned if the verification failed; nil, if verification succeeded.
type ActionVerifierFunc func(action clienttesting.Action) (handled bool, err error)

// ActionAssertion asserts something about the actions that have occurred
type ActionAssertion interface {
	clienttesting.Reactor
	Assert(t *testing.T)
}

// ActionAssertions is simply a typedef for an ActionAssertion slice
type ActionAssertions []ActionAssertion

// AbstractActionFilter serves as a base for building ActionAssertion and
// ActionVerifier types that filter actions based on verb, resource,
// subresource, namespace, and name.
type AbstractActionFilter struct {
	Namespace   string
	Name        string
	Verb        string
	Resource    string
	Subresource string
}

// Handles returns true if the action matches the settings for this verifier
// (verb, resource, subresource, namespace, and name) and the verifier has not
// already been applied.
func (a *AbstractActionFilter) Handles(action clienttesting.Action) bool {
	if (action.Matches(a.Verb, a.Resource) ||
		((a.Verb == "*" || a.Verb == action.GetVerb()) &&
			(a.Resource == "*" || a.Resource == action.GetResource().Resource))) &&
		(a.Subresource == "*" || action.GetSubresource() == a.Subresource) &&
		(a.Namespace == "*" || a.Namespace == action.GetNamespace()) {
		switch typedAction := action.(type) {
		case clienttesting.CreateAction:
			accessor, err := meta.Accessor(typedAction.GetObject())
			return a.Name == "*" || (err == nil && a.Name == accessor.GetName())
		case clienttesting.UpdateAction:
			accessor, err := meta.Accessor(typedAction.GetObject())
			return a.Name == "*" || (err == nil && a.Name == accessor.GetName())
		case clienttesting.DeleteAction:
			return a.Name == "*" || a.Name == typedAction.GetName()
		case clienttesting.GetAction:
			return a.Name == "*" || a.Name == typedAction.GetName()
		case clienttesting.ListAction:
			return true
		case clienttesting.GenericAction:
			return true
		default:
			return true
		}
	}
	return false
}

// On initializes the resource and subresource name to which the created
// filter should apply.  resource parameter should be specified using a slash
// between resource an subresource, e.g. deployments/status.  Use "*" to match
// all resources.
func (a *AbstractActionFilter) On(resource string) *AbstractActionFilter {
    resourceAndSub := strings.SplitN(resource, "/", 2)
    a.Resource = resourceAndSub[0]
    if len(resourceAndSub) > 1 {
        a.Subresource = resourceAndSub[1]
    }
    return a
}

// In initializes the namespace whithin which the filter should apply.  Use "*"
// to match all namespaces.
func (a *AbstractActionFilter) In(namespace string) *AbstractActionFilter {
    a.Namespace = namespace
    return a
}

// Named initializes the name of the resource to which the filter should apply.
// Use "*" to match all names.
func (a *AbstractActionFilter) Named(name string) *AbstractActionFilter {
    a.Name = name
    return a
}

// FilterString returns a sensible string for the filter, e.g. create deployments named namespace-a/some-name
func (a *AbstractActionFilter) String() string {
	if a.Subresource == "" {
		return fmt.Sprintf("%s on %s named %s in %s", a.Verb, a.Resource, a.Name, a.Namespace)
	}
	return fmt.Sprintf("%s on %s/%s named %s in %s", a.Verb, a.Resource, a.Subresource, a.Name, a.Namespace)
}