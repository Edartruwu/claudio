package attachclient

import (
	"github.com/Abraxas-365/claudio/internal/attach"
	"github.com/Abraxas-365/claudio/internal/tools"
)

// AttachScreenshotPusher implements tools.ScreenshotPusher by forwarding
// screenshot events to the ComandCenter server via the attach WebSocket.
// The server copies the file into its uploads directory and pushes an image
// bubble to all browser clients watching the session.
type AttachScreenshotPusher struct {
	client *Client
}

// NewAttachScreenshotPusher creates a pusher backed by the given attach client.
func NewAttachScreenshotPusher(client *Client) *AttachScreenshotPusher {
	return &AttachScreenshotPusher{client: client}
}

// PushScreenshot sends EventDesignScreenshot to the ComandCenter server.
// sessionID is included in the payload as informational context; server-side
// routing uses the hub's session binding established at connect time.
func (p *AttachScreenshotPusher) PushScreenshot(sessionID, filePath, filename string) error {
	return p.client.SendEvent(attach.EventDesignScreenshot, attach.DesignScreenshotPayload{
		FilePath: filePath,
		Filename: filename,
	})
}

// PushBundleLink sends EventDesignBundleReady to the ComandCenter server.
// The server pushes a clickable link bubble to all browser clients watching the session.
func (p *AttachScreenshotPusher) PushBundleLink(sessionID, bundleURL, sessionName string) error {
	return p.client.SendEvent(attach.EventDesignBundleReady, attach.DesignBundlePayload{
		SessionID:   sessionID,
		BundleURL:   bundleURL,
		SessionName: sessionName,
	})
}

// Compile-time interface check.
var _ tools.ScreenshotPusher = (*AttachScreenshotPusher)(nil)
