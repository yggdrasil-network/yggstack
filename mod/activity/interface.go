package activity

// // // // // // // // // //

// CallbackInterface receives connection lifecycle notifications.
// Implementations must not block.
type CallbackInterface interface {
	OnConnectionCreated(connId string, protocol string)
	OnConnectionClosed(connId string)
}
