package api

import (
	"net/http"

	h "caller/internal/api/handlers"
	"caller/internal/config"
	"caller/internal/middleware"

	"github.com/gorilla/websocket"
)

func NewRouter(cfg *config.Config, upgrader websocket.Upgrader) http.Handler {
	mux := http.NewServeMux()

	// Twilio x ElevenLabs
	mux.Handle("/incoming-call", h.HandleIncomingCall())
	mux.Handle("/media-stream", h.HandleMediaStream(upgrader, cfg))
	mux.Handle("/outbound-call", h.HandleOutboundCall(cfg))
	mux.Handle("/outbound-call-twiml", h.HandleOutboundCallTwiml())

	var handler http.Handler = mux
	handler = middleware.Logging(handler)

	return handler
}
