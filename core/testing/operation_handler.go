package testing

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
	"testing"
)

// OperationHandlerTest encapsulates common setup and assertion logic for operation handler tests.
type OperationHandlerTest struct {
	TB      testing.TB
	Ctx     TestContext
	Handler core.OperationHandler
}

// NewOperationHandlerTest creates a new OperationHandlerTest instance.
func NewOperationHandlerTest(ctx TestContext, handler core.OperationHandler) *OperationHandlerTest {
	return &OperationHandlerTest{
		TB:      ctx.T(),
		Ctx:     ctx,
		Handler: handler,
	}
}

// ExecuteRequest executes the operation handler with the given request.
func (oht *OperationHandlerTest) ExecuteRequest(request *models.Request) error {
	return oht.Handler.Execute(oht.Ctx, request)
}

// ValidateRequest validates the request using the operation handler.
func (oht *OperationHandlerTest) ValidateRequest(request *models.Request) error {
	return oht.Handler.ValidateRequest(oht.Ctx, request)
}

// GetStatus gets the current status of the operation.
func (oht *OperationHandlerTest) GetStatus(request *models.Request) (core.RequestStatus, error) {
	return oht.Handler.GetStatus(oht.Ctx, request)
}

// AssertNoError asserts that the error is nil.
func (oht *OperationHandlerTest) AssertNoError(err error) {
	assert.NoError(oht.TB, err)
}

// AssertErrorContains asserts that the error is not nil and contains the given message.
func (oht *OperationHandlerTest) AssertErrorContains(err error, expectedMessage string) {
	assert.Error(oht.TB, err)
	assert.Contains(oht.TB, err.Error(), expectedMessage)
}

// Cleanup invokes the operation handler's cleanup
func (oht *OperationHandlerTest) Cleanup(request *models.Request) error {
	return oht.Handler.Cleanup(oht.Ctx, request)
}

// AssertStatusProgress asserts the operation has specific progress percentage
func (oht *OperationHandlerTest) AssertStatusProgress(request *models.Request, expectedProgress float64) {
	status, err := oht.GetStatus(request)
	oht.AssertNoError(err)
	assert.Equal(oht.TB, expectedProgress, status.ProgressPercent)
}

// AssertStatusMessageContains asserts status message contains substring
func (oht *OperationHandlerTest) AssertStatusMessageContains(request *models.Request, substring string) {
	status, err := oht.GetStatus(request)
	oht.AssertNoError(err)
	assert.Contains(oht.TB, status.Message, substring)
}

// ExecuteAndAssertSuccess executes and asserts no error
func (oht *OperationHandlerTest) ExecuteAndAssertSuccess(request *models.Request) {
	err := oht.ExecuteRequest(request)
	oht.AssertNoError(err)
}

// ExecuteAndAssertError executes and asserts error contains message
func (oht *OperationHandlerTest) ExecuteAndAssertError(request *models.Request, expectedError string) {
	err := oht.ExecuteRequest(request)
	oht.AssertErrorContains(err, expectedError)
}

// ValidateAndAssertSuccess validates and asserts no error
func (oht *OperationHandlerTest) ValidateAndAssertSuccess(request *models.Request) {
	err := oht.ValidateRequest(request)
	oht.AssertNoError(err)
}

// ValidateAndAssertError validates and asserts error contains message  
func (oht *OperationHandlerTest) ValidateAndAssertError(request *models.Request, expectedError string) {
	err := oht.ValidateRequest(request)
	oht.AssertErrorContains(err, expectedError)
}

// CreateTestRequest creates a basic request with operation type using RequestBuilder
func (oht *OperationHandlerTest) CreateTestRequest(proto string, operation string) *models.Request {
	return NewRequest(proto, core.OperationType(operation)).
		Build()
}

// AssertRequestStatus asserts that the request has the expected status.
func (oht *OperationHandlerTest) AssertRequestStatus(request *models.Request, expectedStatus models.RequestStatusType) {
	requestSvc := core.GetService[core.RequestService](oht.Ctx, core.REQUEST_SERVICE)
	currentReq, err := requestSvc.GetRequest(oht.Ctx, request.ID)
	require.NoError(oht.TB, err)
	assert.Equal(oht.TB, expectedStatus, currentReq.Status)
}
