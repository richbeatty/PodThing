package player

import (
    "strings"
    "testing"

    "github.com/richbeatty/PodThing/internal/hardware"
)

func TestStorageDiscoveryDistinguishesUSBAndDiskMode(t *testing.T) {
    roots := []hardware.Device{
        {Name: "usb1", Product: "xHCI Host Controller", Vendor: "1d6b", ProductID: "0002"},
        {Name: "usb2", Product: "xHCI Host Controller", Vendor: "1d6b", ProductID: "0003"},
    }
    err := storageDiscoveryError(roots, "host")
    if err == nil || !strings.Contains(err.Error(), "No external USB devices") {
        t.Fatalf("expected root-only diagnosis, got %v", err)
    }
    if strings.Contains(strings.ToLower(err.Error()), "reconnect it in disk mode") {
        t.Fatalf("no iPod enumerated: must not claim its partition mode: %v", err)
    }
    if s := externalUSBInventory(roots); s != "no downstream USB devices" {
        t.Fatalf("root hubs wrongly presented as external devices: %q", s)
    }
    if err := storageDiscoveryError(roots, "device"); err == nil || !strings.Contains(err.Error(), "Enable USB host") {
        t.Fatalf("expected device role diagnosis: %v", err)
    }
    other := append(append([]hardware.Device{}, roots...), hardware.Device{Product: "USB 2.0 Hub", Vendor: "abcd", ProductID: "1234"})
    if err := storageDiscoveryError(other, "host"); err == nil || !strings.Contains(err.Error(), "no iPod") {
        t.Fatalf("expected missing iPod despite enumerated hub: %v", err)
    }
    apple := append(append([]hardware.Device{}, roots...), hardware.Device{Product: "Apple Mobile Device", Vendor: "05ac", ProductID: "ffff"})
    if err := storageDiscoveryError(apple, "host"); err == nil || !strings.Contains(err.Error(), "Apple USB device") {
        t.Fatalf("expected unrecognized Apple device: %v", err)
    }
    ipod := append(append([]hardware.Device{}, roots...), hardware.Device{Product: "iPod", Vendor: "05ac", ProductID: "1261", IPod: true})
    if err := storageDiscoveryError(ipod, "host"); err != nil {
        t.Fatalf("enumerated single iPod should pass discovery: %v", err)
    }
    both := append(ipod, hardware.Device{Product: "iPod", Vendor: "05ac", ProductID: "1261", IPod: true})
    if err := storageDiscoveryError(both, "host"); err == nil || !strings.Contains(err.Error(), "2 iPods") {
        t.Fatalf("expected multiple iPod safety check: %v", err)
    }
}
