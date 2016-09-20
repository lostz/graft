package graft

//ChanHandler is a convenience handler
type ChanHandler struct {
	// Chan to receive state changes.
	stateChangeChan chan<- StateChange
	// Chan to receive errors.
	errorChan chan<- error
}

// StateChange captures "from" and "to" States for the ChanHandler.
type StateChange struct {
	// From is the previous state.
	From State

	// To is the new state.
	To State
}

//NewChanHandler ....
func NewChanHandler(scCh chan<- StateChange, errCh chan<- error) *ChanHandler {
	return &ChanHandler{
		stateChangeChan: scCh,
		errorChan:       errCh,
	}
}

//StateChange  Queue the state change onto the channel
func (c *ChanHandler) StateChange(from, to State) {
	c.stateChangeChan <- StateChange{From: from, To: to}
}

//AddError Queue the error onto the channel
func (c *ChanHandler) AddError(err error) {
	c.errorChan <- err
}
