package iap

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"
)

// AuthenticationRequest describes the precise challenge without recording
// certificate material, private keys or the challenge bytes.
type AuthenticationRequest struct {
	Command uint16
	Length  int
}
func (e *AuthenticationRequest) Error() string {
	switch e.Command {
	case 0x14:
		return "00/0014: MFi certificate requested"
	case 0x17:
		return fmt.Sprintf("00/0017: MFi signature requested (%d bytes)", e.Length)
	default:
		return fmt.Sprintf("iPod requested authentication (iAP 00/%04x)", e.Command)
	}
}
func (e *AuthenticationRequest) Unwrap() error { return ErrAuthentication }

type Transport interface {
	Send([]byte) error
	Receive(context.Context) ([]byte, error)
	Close() error
}
type Client struct {
	wire       Transport
	log        func(string)
	sampleRate uint32
	simpleHold time.Duration
	identifyLingo byte
}
type Entry struct {
	Index uint32 `json:"index"`
	Name  string `json:"name"`
}
type Selection struct {
	Category byte   `json:"category"`
	Index    uint32 `json:"index"`
	Name     string `json:"name"`
}
type NowPlaying struct {
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Album    string `json:"album"`
	Position uint32 `json:"position_ms"`
	Duration uint32 `json:"duration_ms"`
	Playing  bool   `json:"playing"`
	Shuffle  byte   `json:"shuffle"`
	Repeat   byte   `json:"repeat"`
}

func New(w Transport, log func(string)) *Client {
	if log == nil {
		log = func(string) {}
	}
	return &Client{wire: w, log: log, sampleRate: 44100, simpleHold: 150*time.Millisecond, identifyLingo: 4}
}
func (c *Client) send(p Packet) error {
	c.log(fmt.Sprintf("TX lingo=%02x command=%04x bytes=%d", p.Lingo, p.Command, len(p.Data)))
	return c.wire.Send(Encode(p))
}
func (c *Client) receive(ctx context.Context) (Packet, error) {
	for {
		b, e := c.wire.Receive(ctx)
		if e != nil {
			return Packet{}, e
		}
		p, e := Decode(b)
		if e != nil {
			return Packet{}, e
		}
		c.log(fmt.Sprintf("RX lingo=%02x command=%04x bytes=%d", p.Lingo, p.Command, len(p.Data)))
		if p.Lingo == 0 && p.Command == 0 {
			c.log("iPod requested accessory identification")
			// An IdentifyDeviceLingoes probe must not silently switch
			// the iPod into legacy Extended Interface mode.
			if c.identifyLingo == 0 {
				c.log("iAP 00/0000 RequestIdentify observed during modern probe")
				return p, nil
			}
			if e = c.send(Packet{Lingo: 0, Command: 1, Data: []byte{c.identifyLingo}}); e != nil {
				return Packet{}, e
			}
			continue
		}
		if p.Lingo == 0 && (p.Command == 0x14 || p.Command == 0x17) {
			challenge := &AuthenticationRequest{Command: p.Command, Length: len(p.Data)}
			c.log("AUTH " + challenge.Error())
			return Packet{}, challenge
		}
		if p.Lingo == 0x0a && p.Command == 2 {
			if e = c.send(Packet{Lingo: 0x0a, Command: 3, Data: U32(44100)}); e != nil {
				return Packet{}, e
			}
			continue
		}
		if p.Lingo == 0x0a && p.Command == 4 {
			if len(p.Data) >= 4 {
				c.sampleRate = binary.BigEndian.Uint32(p.Data)
			}
			if e = c.send(Packet{Lingo: 0x0a, Command: 0, Data: []byte{0, 4}}); e != nil {
				return Packet{}, e
			}
			continue
		}
		return p, nil
	}
}
func (c *Client) request(p Packet, response uint16) (Packet, error) {
	if e := c.send(p); e != nil {
		return Packet{}, e
	}
	// Remote mode transitions can respond PENDING before their final ACK.
	timeout := 4 * time.Second
	if p.Lingo == 0 && (p.Command == 3 || p.Command == 5 || p.Command == 6) {
		timeout = 8 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for {
		r, e := c.receive(ctx)
		if e != nil {
			return Packet{}, fmt.Errorf("iAP %02x/%04x: %w", p.Lingo, p.Command, e)
		}
		ack := (r.Lingo == 0 && r.Command == 2 && len(r.Data) >= 2 && uint16(r.Data[1]) == p.Command && p.Lingo == 0) || (r.Lingo == 4 && r.Command == 1 && len(r.Data) >= 3 && binary.BigEndian.Uint16(r.Data[1:]) == p.Command && p.Lingo == 4)
		if ack {
			status := r.Data[0]
			if status == 6 {
				c.log(fmt.Sprintf("iAP %02x/%04x ACK pending", p.Lingo, p.Command))
				continue
			}
			if status == 7 {
				c.log(fmt.Sprintf("AUTH iPod rejected request %02x/%04x: not authenticated (ACK 0x07)", p.Lingo, p.Command))
				return Packet{}, fmt.Errorf("%w: request %02x/%04x rejected as not authenticated (ACK 0x07)", ErrAuthentication, p.Lingo, p.Command)
			}
			if status != 0 {
				return Packet{}, fmt.Errorf("iPod rejected %02x/%04x: status 0x%02x", p.Lingo, p.Command, status)
			}
			if response == 0 {
				return r, nil
			}
			continue
		}
		if response != 0 && r.Lingo == p.Lingo && r.Command == response {
			return r, nil
		}
	}
}
func (c *Client) Handshake() (string, error) {
	// Identify is send-only. Some iPods automatically enter Extended Interface
	// when they see lingo 04; others require an explicit EnterRemoteUIMode.
	c.log("STAGE iAP1: identifying accessory (Extended Interface)")
	if e := c.send(Packet{Lingo: 0, Command: 1, Data: []byte{4}}); e != nil {
		return "", fmt.Errorf("iAP identification transfer: %w", e)
	}
	c.log("STAGE iAP1: checking remote UI mode")
	remote := false
	mode, err := c.request(Packet{Lingo: 0, Command: 3}, 4)
	if err == nil {
		if len(mode.Data) < 1 {
			return "", errors.New("iAP remote mode response contained no state byte")
		}
		remote = mode.Data[0] != 0
		c.log(fmt.Sprintf("Remote UI mode reported: %t", remote))
	} else if errors.Is(err, ErrAuthentication) {
		return "", err
	} else {
		// Older firmware may not answer RequestRemoteUIMode. Do not mistake
		// a missing reply for success; explicitly enter the mode below.
		c.log("Remote UI query unavailable; trying explicit mode entry: " + err.Error())
	}
	if !remote {
		c.log("STAGE iAP1: entering remote UI mode")
		if _, err = c.request(Packet{Lingo: 0, Command: 5}, 0); err != nil {
			return "", fmt.Errorf("iAP enter remote UI mode: %w", err)
		}
	}
	// A name is useful but is not an authentication or mode requirement.
	c.log("STAGE iAP1: reading iPod name")
	name, err := c.request(Packet{Lingo: 0, Command: 7}, 8)
	if err != nil {
		if errors.Is(err, ErrAuthentication) {
			return "", err
		}
		c.log("iPod name unavailable (remote mode succeeded): " + err.Error())
		return "iPod", nil
	}
	if s := text(name.Data); s != "" {
		return s, nil
	}
	return "iPod", nil
}
func (c *Client) Close() error {
	_, _ = c.request(Packet{Lingo: 0, Command: 6}, 0)
	// Deliberate disconnect restores the original USB configuration.
	// Aborting a failed handshake must not cause a mass-storage bounce.
	if r, ok := c.wire.(interface{ RestoreOriginalOnClose() }); ok {
		r.RestoreOriginalOnClose()
	}
	return c.wire.Close()
}
func (c *Client) Abort() error { return c.wire.Close() }

// BeginSimpleRemote is an explicit compatibility test, not MFi bypass.
// Legacy Identify with lingo 2 resets Extended Interface negotiation; it
// advertises only the simple remote control capability. No ACK is expected.
func (c *Client) BeginSimpleRemote() error {
	c.identifyLingo = 2
	c.log("STAGE iAP1: trying legacy Simple Remote, no MFi capabilities")
	if err := c.send(Packet{Lingo: 0, Command: 1, Data: []byte{2}}); err != nil {
		return fmt.Errorf("iAP Simple Remote Identify: %w", err)
	}
	return nil
}

// SimpleRemoteKey sends an unacknowledged button press and a separate release.
// A successful transfer proves only that Linux sent both packets, not that the
// stock iPod accepted them. iAP1 Simple Remote lingo 02 command 00.
func (c *Client) SimpleRemoteKey(key string) error {
	mask := byte(0)
	switch key {
	case "play":
		mask = 0x01 // play / pause key
	case "next":
		mask = 0x08 // right / next
	case "previous":
		mask = 0x10 // left / previous
	default:
		return fmt.Errorf("unsupported Simple Remote key %q", key)
	}
	c.log("STAGE iAP1: sending Simple Remote " + key + " (unacknowledged)")
	if err := c.send(Packet{Lingo: 2, Command: 0, Data: []byte{mask}}); err != nil {
		return fmt.Errorf("Simple Remote %s press: %w", key, err)
	}
	// V5 released immediately. Give the iPod time to sample the key.
	if c.simpleHold > 0 { time.Sleep(c.simpleHold) }
	if err := c.send(Packet{Lingo: 2, Command: 0, Data: []byte{0}}); err != nil {
		return fmt.Errorf("Simple Remote %s release: %w", key, err)
	}
	return nil
}

// ProbeAccessory follows the modern IdentifyDeviceLingoes method described
// by the pi-iap-bt-bridge test stereo, but without pretending to possess
// credentials: General, Simple, Display and Extended, no Digital Audio,
// no authentication flags, and no device ID.
func (c *Client) ProbeAccessory() (string, error) {
	c.identifyLingo = 0
	c.log("STAGE iAP1: modern IdentifyDeviceLingoes, no MFi credentials")
	data := append(U32(0x1d), U32(0)...)
	data = append(data, U32(0)...)
	if _, err := c.request(Packet{Lingo: 0, Command: 0x13, Data: data}, 0); err != nil {
		return "", fmt.Errorf("modern IdentifyDeviceLingoes: %w", err)
	}
	c.log("STAGE iAP1: modern identification accepted; querying name")
	name, err := c.request(Packet{Lingo: 0, Command: 7}, 8)
	if err != nil { return "", fmt.Errorf("modern accessory name: %w", err) }
	result := text(name.Data)
	if result == "" { result = "iPod (unnamed)" }
	c.log("STAGE iAP1: querying remote state without entering remote mode")
	mode, err := c.request(Packet{Lingo: 0, Command: 3}, 4)
	if err != nil { return result, fmt.Errorf("modern accessory remote state: %w", err) }
	if len(mode.Data) == 0 { return result, errors.New("remote mode response lacked state") }
	c.log(fmt.Sprintf("Modern accessory responded; Extended UI active=%t", mode.Data[0] != 0))
	return result, nil
}

// RequestDigitalAudio advertises USB digital-audio support explicitly. A real
// authentication challenge is returned to the UI, never answered with fake data.
func (c *Client) RequestDigitalAudio() error {
	data := append(U32(1|1<<4|1<<10), U32(0)...)
	data = append(data, U32(0)...)
	_, e := c.request(Packet{Lingo: 0, Command: 0x13, Data: data}, 0)
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	for {
		_, e = c.receive(ctx)
		if errors.Is(e, context.DeadlineExceeded) {
			return nil
		}
		if e != nil {
			return e
		}
	}
}
func (c *Client) Browse(path []Selection, cat byte, offset, limit uint32) ([]Entry, uint32, error) {
	if limit == 0 || limit > 50 {
		return nil, 0, errors.New("page size must be 1..50")
	}
	if _, e := c.request(Packet{Lingo: 4, Command: 0x16}, 0); e != nil {
		return nil, 0, e
	}
	for _, s := range path {
		if _, e := c.request(Packet{Lingo: 4, Command: 0x17, Data: append([]byte{s.Category}, U32(s.Index)...)}, 0); e != nil {
			return nil, 0, e
		}
	}
	p, e := c.request(Packet{Lingo: 4, Command: 0x18, Data: []byte{cat}}, 0x19)
	if e != nil {
		return nil, 0, e
	}
	if len(p.Data) != 4 {
		return nil, 0, errors.New("invalid category-count response")
	}
	total := binary.BigEndian.Uint32(p.Data)
	if offset >= total {
		return []Entry{}, total, nil
	}
	n := min(limit, total-offset)
	data := append([]byte{cat}, U32(offset)...)
	data = append(data, U32(n)...)
	if e = c.send(Packet{Lingo: 4, Command: 0x1a, Data: data}); e != nil {
		return nil, total, e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	entries := make([]Entry, 0, n)
	seen := map[uint32]bool{}
	for uint32(len(entries)) < n {
		r, e := c.receive(ctx)
		if e != nil {
			return nil, total, e
		}
		if r.Lingo != 4 {
			continue
		}
		if r.Command == 1 && len(r.Data) >= 3 && binary.BigEndian.Uint16(r.Data[1:]) == 0x1a && r.Data[0] != 0 {
			return nil, total, fmt.Errorf("browse rejected: status 0x%02x", r.Data[0])
		}
		if r.Command != 0x1b {
			continue
		}
		if len(r.Data) < 5 {
			return nil, total, errors.New("short database record")
		}
		idx := binary.BigEndian.Uint32(r.Data)
		if idx < offset || idx >= offset+n || seen[idx] {
			continue
		}
		seen[idx] = true
		entries = append(entries, Entry{idx, text(r.Data[4:])})
	}
	return entries, total, nil
}
func (c *Client) Play(index uint32) error {
	_, e := c.request(Packet{Lingo: 4, Command: 0x28, Data: U32(index)}, 0)
	return e
}
func (c *Client) Control(code byte) error {
	_, e := c.request(Packet{Lingo: 4, Command: 0x29, Data: []byte{code}}, 0)
	return e
}
func (c *Client) SetShuffle(v byte) error {
	_, e := c.request(Packet{Lingo: 4, Command: 0x2e, Data: []byte{v, 0}}, 0)
	return e
}
func (c *Client) SetRepeat(v byte) error {
	_, e := c.request(Packet{Lingo: 4, Command: 0x31, Data: []byte{v, 0}}, 0)
	return e
}
func (c *Client) Now() (NowPlaying, error) {
	n := NowPlaying{}
	p, e := c.request(Packet{Lingo: 4, Command: 0x1c}, 0x1d)
	if e != nil {
		return n, e
	}
	if len(p.Data) != 9 {
		return n, errors.New("invalid play-status response")
	}
	n.Duration = binary.BigEndian.Uint32(p.Data)
	n.Position = binary.BigEndian.Uint32(p.Data[4:])
	n.Playing = p.Data[8] == 1
	p, e = c.request(Packet{Lingo: 4, Command: 0x1e}, 0x1f)
	if e != nil {
		return n, e
	}
	if len(p.Data) != 4 {
		return n, errors.New("invalid track-index response")
	}
	idx := append([]byte(nil), p.Data...)
	for _, q := range []struct {
		cmd uint16
		dst *string
	}{{0x20, &n.Title}, {0x22, &n.Artist}, {0x24, &n.Album}} {
		p, e = c.request(Packet{Lingo: 4, Command: q.cmd, Data: idx}, q.cmd+1)
		if e != nil {
			return n, e
		}
		*q.dst = text(p.Data)
	}
	for _, q := range []struct {
		cmd uint16
		dst *byte
	}{{0x2c, &n.Shuffle}, {0x2f, &n.Repeat}} {
		p, e = c.request(Packet{Lingo: 4, Command: q.cmd}, q.cmd+1)
		if e != nil {
			return n, e
		}
		if len(p.Data) < 1 {
			return n, errors.New("short playback-mode response")
		}
		*q.dst = p.Data[0]
	}
	return n, nil
}
func text(b []byte) string {
	s := strings.SplitN(string(b), "\x00", 2)[0]
	return strings.ToValidUTF8(s, "?")
}
