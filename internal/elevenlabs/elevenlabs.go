package elevenlabs

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// GetSignedElevenLabsURL retrieves a signed WebSocket URL from ElevenLabs
func GetSignedElevenLabsURL(agentID string, apiKey string) (string, error) {
	url := fmt.Sprintf("https://api.elevenlabs.io/v1/convai/conversation/get_signed_url?agent_id=%s", agentID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("error creating signed URL request: %w", err)
	}

	req.Header.Set("xi-api-key", apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error getting signed URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get signed URL: %s", resp.Status)
	}

	var result struct {
		SignedURL string `json:"signed_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("error parsing signed URL response: %w", err)
	}

	return result.SignedURL, nil
}

// GenerateElevenLabsConfig creates configuration for initializing ElevenLabs conversation
func GenerateElevenLabsConfig(userData map[string]interface{}, callerPhone string, isInbound bool) map[string]interface{} {
	config := map[string]interface{}{
		"type": "conversation_initiation_client_data",
		"conversation_config_override": map[string]interface{}{
			"agent": map[string]interface{}{},
		},
	}

	var firstName, lastName string

	if userData != nil {
		if debtor, ok := userData["debtor"].(map[string]interface{}); ok {
			if fn, ok := debtor["first_name"].(string); ok {
				firstName = fn
			}
			if ln, ok := debtor["last_name"].(string); ok {
				lastName = ln
			}
		}
	}

	// Set prompt and first message based on call direction
	if isInbound {
		// For inbound calls
		inboundPrompt, _ := generateInboundCallPrompt(
			fmt.Sprintf("%s %s", firstName, lastName),
		)

		agentConfig := config["conversation_config_override"].(map[string]interface{})["agent"].(map[string]interface{})
		agentConfig["prompt"] = map[string]interface{}{
			"prompt": inboundPrompt,
		}
		agentConfig["first_message"] = fmt.Sprintf("Hi %s! I'm a supportive AI Agent. Do you have a moment to talk?", firstName)
	} else {
		// For outbound calls
		outboundPrompt, _ := generateOutboundCallPrompt(
			fmt.Sprintf("%s %s", firstName, lastName),
		)

		agentConfig := config["conversation_config_override"].(map[string]interface{})["agent"].(map[string]interface{})
		agentConfig["prompt"] = map[string]interface{}{
			"prompt": outboundPrompt,
		}
		agentConfig["first_message"] = fmt.Sprintf("Hi %s! A supportive AI Agent is here to help you. What do you want to talk about?", firstName)
	}

	// add dynamic variables if user data is available
	if userData != nil {
		config["client_data"] = map[string]interface{}{
			"dynamic_variables": map[string]string{
				"caller_phone": callerPhone,
				"caller_name":  fmt.Sprintf("%s %s", firstName, lastName),
			},
		}
	}

	return config
}

func generateInboundCallPrompt(
	name string,
) (string, error) {
	prompt := `
You are an AI Agent that is supportive and helpful.
You main task it to motivate the interlocutor who's name is %s to enjoy their life.
`

	return fmt.Sprintf(prompt, name), nil
}

func generateOutboundCallPrompt(
	name string,
) (string, error) {
	prompt := `
You are an AI Agent that is supportive and helpful.
You main task it to motivate the interlocutor who's name is %s to enjoy their life.
`

	return fmt.Sprintf(prompt, name), nil
}
