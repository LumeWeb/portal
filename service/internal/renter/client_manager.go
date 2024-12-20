package renter

import (
	"context"
	"encoding/json"
	"fmt"
	"go.etcd.io/etcd/client/v3"
	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
	"sync"
	"time"
)

type ClientType string

const (
	ClientTypeBus    ClientType = "bus"
	ClientTypeWorker ClientType = "worker"
	ClusterKey       string     = "/discovery/renterd"
)

const (
	nodeTTL = 30 * time.Second // Time after which a node is considered dead if not updated
)

type NodeInfo struct {
	URL       string     `json:"url"`
	Type      ClientType `json:"type"`
	LastSeen  time.Time  `json:"last_seen"`
	Priority  int        `json:"priority"`
	IsHealthy bool       `json:"is_healthy"`
}

type ClientManager struct {
	ctx       core.Context
	etcdKey   string
	nodes     map[ClientType][]NodeInfo
	nodesLock sync.RWMutex
	logger    *core.Logger
}

func NewClientManager(ctx core.Context) *ClientManager {
	return &ClientManager{
		ctx:     ctx,
		etcdKey: ctx.Config().Config().Core.Clustered.Etcd.ComputePrefix(ClusterKey),
		nodes:   make(map[ClientType][]NodeInfo),
		logger:  ctx.Logger(),
	}
}

func (cm *ClientManager) Start() error {
	if !cm.ctx.Config().Config().Core.ClusterEnabled() {
		return nil
	}

	client, err := cm.ctx.Config().Config().Core.Clustered.Etcd.Client()
	if err != nil {
		return fmt.Errorf("failed to create etcd client: %w", err)
	}

	// Initial load of nodes
	if err := cm.loadNodes(client); err != nil {
		return err
	}

	// Watch for changes
	go cm.watchNodes(client)

	return nil
}

func (cm *ClientManager) loadNodes(client *clientv3.Client) error {
	resp, err := client.Get(context.Background(), cm.etcdKey, clientv3.WithPrefix())
	if err != nil {
		return fmt.Errorf("failed to get nodes from etcd: %w", err)
	}

	cm.nodesLock.Lock()
	defer cm.nodesLock.Unlock()

	for _, kv := range resp.Kvs {
		var node NodeInfo
		if err := json.Unmarshal(kv.Value, &node); err != nil {
			cm.logger.Error("failed to unmarshal node info", zap.Error(err))
			continue
		}
		cm.nodes[node.Type] = append(cm.nodes[node.Type], node)
	}

	return nil
}

func (cm *ClientManager) watchNodes(client *clientv3.Client) {
	watchChan := client.Watch(context.Background(), cm.etcdKey, clientv3.WithPrefix())
	for watchResp := range watchChan {
		for _, event := range watchResp.Events {
			var node NodeInfo
			if err := json.Unmarshal(event.Kv.Value, &node); err != nil {
				cm.logger.Error("failed to unmarshal node info from watch event", zap.Error(err))
				continue
			}

			cm.nodesLock.Lock()
			switch event.Type {
			case clientv3.EventTypePut:
				cm.updateNode(node)
			case clientv3.EventTypeDelete:
				cm.removeNode(node)
			}
			cm.nodesLock.Unlock()
		}
	}
}

func (cm *ClientManager) updateNode(node NodeInfo) {
	nodes := cm.nodes[node.Type]
	node.LastSeen = time.Now()
	for i, existing := range nodes {
		if existing.URL == node.URL {
			nodes[i] = node
			return
		}
	}
	cm.nodes[node.Type] = append(cm.nodes[node.Type], node)
}
func (cm *ClientManager) updateNodeLastUsed(node *NodeInfo) {
	if !cm.ctx.Config().Config().Core.ClusterEnabled() {
		return
	}

	client, err := cm.ctx.Config().Config().Core.Clustered.Etcd.Client()
	if err != nil {
		cm.logger.Error("failed to get etcd client for updating node last used time", zap.Error(err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	nodeKey := fmt.Sprintf("%s/%s", cm.etcdKey, node.URL)
	data, err := json.Marshal(node)
	if err != nil {
		cm.logger.Error("failed to marshal node info", zap.Error(err))
		return
	}

	// Just use a short lease - we only need it for the update
	lease, err := client.Grant(ctx, 1) // 1 second lease
	if err != nil {
		cm.logger.Error("failed to create lease", zap.Error(err))
		return
	}

	_, err = client.Put(ctx, nodeKey, string(data), clientv3.WithLease(lease.ID))
	if err != nil {
		cm.logger.Error("failed to update node last used time in etcd", zap.Error(err))
		// Revoke the lease to clean up
		if _, revokeErr := client.Revoke(ctx, lease.ID); revokeErr != nil {
			cm.logger.Error("failed to revoke lease after PUT failure", zap.Error(revokeErr))
		}
		return
	}
}

func (cm *ClientManager) removeNode(node NodeInfo) {
	nodes := cm.nodes[node.Type]
	for i, existing := range nodes {
		if existing.URL == node.URL {
			cm.nodes[node.Type] = append(nodes[:i], nodes[i+1:]...)
			return
		}
	}
}

func (cm *ClientManager) GetNextNode(clientType ClientType) (*NodeInfo, error) {
	cm.nodesLock.Lock()
	nodes := cm.nodes[clientType]
	if len(nodes) == 0 {
		cm.nodesLock.Unlock()
		return nil, fmt.Errorf("no available %s nodes", clientType)
	}

	now := time.Now()
	var selectedNode *NodeInfo

	// Just find a healthy node - no need to track LastUsed locally
	for i := range nodes {
		if now.Sub(nodes[i].LastSeen) > nodeTTL {
			nodes[i].IsHealthy = false
			continue
		}

		if nodes[i].IsHealthy {
			selectedNode = &nodes[i]
			break
		}
	}
	cm.nodesLock.Unlock()

	if selectedNode == nil {
		return nil, fmt.Errorf("no healthy %s nodes available", clientType)
	}

	return selectedNode, nil
}
