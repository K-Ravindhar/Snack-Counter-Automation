package integrations

import "fmt"

// Chat is the boundary to the company's internal chat tool.
type Chat interface {
	Notify(userID, message string) error
	Broadcast(message string) error
}

type MockChat struct{ Messages []string }

func (m *MockChat) Notify(userID, message string) error {
	m.Messages = append(m.Messages, fmt.Sprintf("DM to %s: %s", userID, message))
	return nil
}
func (m *MockChat) Broadcast(message string) error {
	m.Messages = append(m.Messages, "CHANNEL: "+message)
	return nil
}
