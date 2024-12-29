package config

import (
    "context"
    "time"
    clientv3 "go.etcd.io/etcd/client/v3"
    "go.uber.org/zap"
)

type EtcdManager struct {
    client     *clientv3.Client
    logger     *zap.Logger
    ctx        context.Context
    cancelFunc context.CancelFunc
}

func NewEtcdManager(config *EtcdConfig, logger *zap.Logger) (*EtcdManager, error) {
    ctx, cancel := context.WithCancel(context.Background())
    
    client, err := clientv3.New(clientv3.Config{
        Endpoints:   config.Endpoints,
        DialTimeout: time.Duration(config.DialTimeout) * time.Second,
        Username:    config.Username,
        Password:    config.Password,
    })
    if err != nil {
        cancel()
        return nil, err
    }

    return &EtcdManager{
        client:     client,
        logger:     logger,
        ctx:        ctx,
        cancelFunc: cancel,
    }, nil
}

func (m *EtcdManager) Client() *clientv3.Client {
    return m.client
}

func (m *EtcdManager) Close() error {
    m.cancelFunc()
    if m.client != nil {
        return m.client.Close()
    }
    return nil
}
