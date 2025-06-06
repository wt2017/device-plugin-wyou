package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path"
	"time"

	"google.golang.org/grpc"
	registerapi "k8s.io/kubelet/pkg/apis/pluginregistration/v1"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

const (
	resourceName = "wyou/mrvl-ptp"
	sockDir = "/var/lib/kubelet/plugins_registry"
	socketName   = "wyou-mrvl-ptp.sock"
	devicePath   = "/dev/mrvl_ptp0"
)

const (
	rsWatchInterval    = 5 * time.Second
	serverStartTimeout = 5 * time.Second
)

// MyCharDevicePlugin implements the Kubernetes Device Plugin API.
type MyCharDevicePlugin struct {
	devices 			[]*pluginapi.Device
	stop    			chan struct{}
	socketPath         	string
}

// NewMyCharDevicePlugin initializes a new MyCharDevicePlugin.
func NewMyCharDevicePlugin() *MyCharDevicePlugin {
	return &MyCharDevicePlugin{
		devices: []*pluginapi.Device{
			{
				ID:     "mrvl_ptp_id_0",
				Health: pluginapi.Healthy,
			},
		},
		stop: make(chan struct{}),
	}
}

// Start starts the gRPC server and registers the device plugin with the kubelet.
func (m *MyCharDevicePlugin) Start() error {
    // Create the socket file
    m.socketPath = path.Join(sockDir, socketName)
	log.Printf("Socket path: %s", m.socketPath)
    if err := os.Remove(m.socketPath); err != nil && !os.IsNotExist(err) {
        return fmt.Errorf("failed to remove existing socket: %v", err)
    }

    listener, err := net.Listen("unix", m.socketPath)
    if err != nil {
        return fmt.Errorf("failed to listen on socket: %v", err)
    }

    // gRPC server
    server := grpc.NewServer()

    // Register all services
	registerapi.RegisterRegistrationServer(server, m)
	pluginapi.RegisterDevicePluginServer(server, m)

	go func() {
        if err := server.Serve(listener); err != nil {
            log.Fatalf("Failed to start gRPC server: %v", err)
        }
    }()

	// Wait for server to start by launching a blocking connection
	ctx, cancel := context.WithTimeout(context.TODO(), serverStartTimeout)
	defer cancel()
	conn, err := grpc.DialContext(ctx, "unix:"+m.socketPath, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatalf("Unable to establish test connection with gRPC server")
		return err
	}
	log.Println("Start device plugin endpoint")
	conn.Close()

    return nil
}

// Stop stops the gRPC server.
func (m *MyCharDevicePlugin) Stop() {
	close(m.stop)
}

func (m *MyCharDevicePlugin) GetInfo(ctx context.Context, rqt *registerapi.InfoRequest) (*registerapi.PluginInfo, error) {
	log.Println("GetInfo is invoked\n")
	pluginInfoResponse := &registerapi.PluginInfo{
		Type:              registerapi.DevicePlugin,
		Name:              resourceName,
		Endpoint:          m.socketPath,
		SupportedVersions: []string{"v1alpha1", "v1beta1"},
	}
	return pluginInfoResponse, nil
}

func (m *MyCharDevicePlugin) NotifyRegistrationStatus(ctx context.Context,
	regstat *registerapi.RegistrationStatus,
) (*registerapi.RegistrationStatusResponse, error) {
	if regstat.PluginRegistered {
		log.Println("Plugin: gets registered successfully at Kubelet\n")
	} else {
		log.Fatalf("Plugin: failed to be registered at Kubelet\n")
	}
	return &registerapi.RegistrationStatusResponse{}, nil
}

// ListAndWatch streams the list of devices to the kubelet.
func (m *MyCharDevicePlugin) ListAndWatch(e *pluginapi.Empty, s pluginapi.DevicePlugin_ListAndWatchServer) error {
	log.Println("ListAndWatch is invoked\n")
	for {
		select {
		case <-m.stop:
			return nil
		default:
			s.Send(&pluginapi.ListAndWatchResponse{Devices: m.devices})
			time.Sleep(5 * time.Second)
		}
	}
}

// Allocate allocates devices to containers.
func (m *MyCharDevicePlugin) Allocate(ctx context.Context, req *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	responses := &pluginapi.AllocateResponse{}
	for _, _ = range req.ContainerRequests {
		response := &pluginapi.ContainerAllocateResponse{
			Devices: []*pluginapi.DeviceSpec{
				{
					ContainerPath: devicePath,
					HostPath:      devicePath,
					Permissions:   "rw",
				},
			},
		}
		responses.ContainerResponses = append(responses.ContainerResponses, response)
	}
	return responses, nil
}

// GetDevicePluginOptions returns options for the device plugin.
func (m *MyCharDevicePlugin) GetDevicePluginOptions(ctx context.Context, empty *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	return &pluginapi.DevicePluginOptions{
		PreStartRequired: false,
	}, nil
}

func (m *MyCharDevicePlugin) GetPreferredAllocation(ctx context.Context, par *pluginapi.PreferredAllocationRequest) (*pluginapi.PreferredAllocationResponse, error) {
	log.Println("GetPreferredAllocation() called with %+v", par)
	resp := new(pluginapi.PreferredAllocationResponse)
	return resp, nil
}

func (m *MyCharDevicePlugin) PreStartContainer(ctx context.Context,
	psRqt *pluginapi.PreStartContainerRequest,
) (*pluginapi.PreStartContainerResponse, error) {
	return &pluginapi.PreStartContainerResponse{}, nil
}

func init() {
    log.SetOutput(os.Stdout)
    log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func main() {
	log.Println("Start plugin")
	plugin := NewMyCharDevicePlugin()
	if err := plugin.Start(); err != nil {
		log.Fatalf("Failed to start device plugin: %v", err)
	}
	defer plugin.Stop()

	// Keep the plugin running
	select {}
}
