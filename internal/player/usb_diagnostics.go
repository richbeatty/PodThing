package player

import (
    "errors"
    "fmt"
    "sort"
    "strings"

    "github.com/richbeatty/PodThing/internal/hardware"
)

// The Linux USB root hubs are always present, even when nothing downstream
// has negotiated a data connection. They are not evidence of a working OTG hub.
func isRootUSBController(d hardware.Device) bool {
    return d.Vendor == "1d6b" &&
        strings.HasPrefix(d.Name, "usb") &&
        (d.ProductID == "0002" || d.ProductID == "0003")
}

// externalUSBInventory deliberately omits serial numbers and sysfs paths.
// It records only product names and USB vendor/product IDs for safe reports.
func externalUSBInventory(devices []hardware.Device) string {
    names := make([]string, 0, len(devices))
    for _, d := range devices {
        if isRootUSBController(d) { continue }
        names = append(names, fmt.Sprintf("%s (%s:%s)", d.Product, d.Vendor, d.ProductID))
    }
    sort.Strings(names)
    if len(names) == 0 { return "no downstream USB devices" }
    if len(names) > 20 {
        return fmt.Sprintf("%s and %d more", strings.Join(names[:20], ", "), len(names)-20)
    }
    return strings.Join(names, ", ")
}

// A disk-mode or missing-partition suggestion is only appropriate *after*
// an iPod enumerates. First distinguish a missing USB data connection.
func storageDiscoveryError(devices []hardware.Device, role string) error {
    count := 0
    var external []hardware.Device
    apple := false
    for _, d := range devices {
        if d.IPod { count++ }
        if !isRootUSBController(d) {
            external = append(external, d)
            if strings.EqualFold(d.Vendor, "05ac") { apple = true }
        }
    }
    if count == 1 { return nil }
    if count > 1 {
        return fmt.Errorf("found %d iPods; connect exactly one iPod using the powered host hub", count)
    }
    if role != "host" {
        return fmt.Errorf("USB is in %q mode, not host mode; select Enable USB host before opening the iPod library", role)
    }
    if len(external) == 0 {
        return errors.New("No external USB devices detected, only the host controllers. Check powered OTG hub, hub power and USB data cable. PodThing cannot check iPod disk mode until a USB device appears")
    }
    if apple {
        return errors.New("An Apple USB device was detected but not identified as a supported iPod. Save the USB diagnostic report with its vendor/product IDs before attempting disk mode")
    }
    return fmt.Errorf("%d external USB device(s) detected but no iPod. Check the 30-pin data cable, iPod power, and hub connection before trying disk mode", len(external))
}
