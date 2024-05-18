package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
)

func ip2int(ip string) uint32 {

	split := strings.Split(ip, ".")
	p1, _ := strconv.Atoi(split[0])
	p2, _ := strconv.Atoi(split[1])
	p3, _ := strconv.Atoi(split[2])
	p4, _ := strconv.Atoi(split[3])

	// 1.2.3.4
	//      byte4                   byte3                         byte2                     byte1
	// iph->saddr >> 24, (iph->saddr & 0x00FF0000) >> 16, (iph->saddr & 0xFF00) >> 8, iph->saddr & 0xFF
	// The order has to be reversed since machine is little endian
	return uint32(p4<<24 | p3<<16 | p2<<8 | p1)
}

func main() {
	// Remove resource limits for kernels <5.11.
	if err := rlimit.RemoveMemlock(); err != nil {
		log.Fatal("Removing memlock:", err)
	}

	// Load the compiled eBPF ELF and load it into the kernel.
	var objs counterObjects
	if err := loadCounterObjects(&objs, nil); err != nil {
		log.Fatal("Loading eBPF objects:", err)
	}
	defer objs.Close()

	ifname := "eth0" // Change this to an interface on your machine.
	iface, err := net.InterfaceByName(ifname)
	if err != nil {
		log.Fatalf("Getting interface %s: %s", ifname, err)
	}

	// Attach count_packets to the network interface.
	link, err := link.AttachXDP(link.XDPOptions{
		Program:   objs.XdpPass,
		Interface: iface.Index,
	})
	if err != nil {
		log.Fatal("Attaching XDP:", err)
	}
	defer link.Close()

	log.Printf("Counting incoming packets on %s..", ifname)

	// Periodically fetch the packet counter from PktCount,
	// exit the program when interrupted.
	tick := time.Tick(time.Second)
	stop := make(chan os.Signal, 5)
	signal.Notify(stop, os.Interrupt)
	for {
		select {
		case <-tick:
			var count uint64

			target := "142.251.40.196"

			targetKey := ip2int(target)

			err := objs.PktCount.Lookup(targetKey, &count)
			if err != nil {
				log.Printf("Waiting for first packet... %s", err)
			} else {
				log.Printf("Received %d packets", count)
			}
		case <-stop:
			log.Print("Received signal, exiting..")
			return
		}
	}
}
