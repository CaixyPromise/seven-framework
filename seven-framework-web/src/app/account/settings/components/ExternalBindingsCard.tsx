'use client';

import React from 'react';
import { Button, Card, List, Space, Tag, Typography } from 'antd';
import {
  GithubOutlined,
  GoogleOutlined,
  LinkOutlined,
  CheckCircleFilled,
  ClockCircleOutlined,
} from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import {
  listCurrentExternalBindings,
  type CurrentExternalBinding,
} from '@/api/externalLoginController';
import PixelAvatar from '@/components/user/PixelAvatar';

const { Text } = Typography;

function providerIcon(providerCode: string) {
  if (providerCode === 'github') {
    return <GithubOutlined />;
  }
  if (providerCode === 'google') {
    return <GoogleOutlined />;
  }
  return <LinkOutlined />;
}

function formatTime(value?: string | null) {
  if (!value) {
    return '尚未使用';
  }
  return new Date(value).toLocaleString();
}

function buildBindUrl(item: CurrentExternalBinding) {
  const base = item.bindUrl || `/external-login/me/${item.providerCode}/start`;
  const normalized = base.startsWith('/api/') ? base : `/api${base.startsWith('/') ? base : `/${base}`}`;
  const url = new URL(normalized, window.location.origin);
  url.searchParams.set('redirectAfterLogin', '/account/settings');
  return `${url.pathname}${url.search}`;
}

export default function ExternalBindingsCard() {
  const { data = [], isLoading } = useQuery({
    queryKey: ['account-settings', 'external-bindings'],
    queryFn: () => listCurrentExternalBindings(),
  });

  const handleBind = (item: CurrentExternalBinding) => {
    window.location.assign(buildBindUrl(item));
  };

  return (
    <Card
      title="第三方账号"
      variant="borderless"
      className="shadow-sm rounded-xl"
      extra={
        <Text type="secondary" className="text-xs">
          绑定后可使用外部平台快速登录
        </Text>
      }
    >
      <List
        loading={isLoading}
        dataSource={data}
        rowKey={(item) => item.providerCode}
        locale={{ emptyText: '暂无可绑定的第三方登录方式' }}
        renderItem={(item) => (
          <List.Item
            className="!px-0"
            actions={[
              item.bound ? (
                <Tag key="bound" color="success" icon={<CheckCircleFilled />}>
                  已绑定
                </Tag>
              ) : (
                <Button
                  key="bind"
                  type="primary"
                  icon={<LinkOutlined />}
                  disabled={!item.bindEnabled}
                  onClick={() => handleBind(item)}
                >
                  绑定
                </Button>
              ),
            ]}
          >
            <List.Item.Meta
              avatar={
                item.bound && item.avatarUrl ? (
                  <PixelAvatar size={44} src={item.avatarUrl} seed={item.externalLogin || item.providerCode} />
                ) : (
                  <div className="h-11 w-11 rounded-xl bg-slate-50 border border-slate-200 flex items-center justify-center text-xl text-slate-700">
                    {providerIcon(item.providerCode)}
                  </div>
                )
              }
              title={
                <Space size={8} wrap>
                  <span className="font-medium text-slate-900">{item.displayName || item.providerCode}</span>
                  {item.bound && item.emailVerified ? <Tag color="blue">邮箱已验证</Tag> : null}
                </Space>
              }
              description={
                item.bound ? (
                  <div className="space-y-1">
                    <div className="text-slate-500">
                      {item.externalLogin || item.externalEmail || '已绑定外部账号'}
                    </div>
                    <div className="text-xs text-slate-400 flex items-center gap-1">
                      <ClockCircleOutlined />
                      最近同步：{formatTime(item.lastVerifiedAt || item.lastLoginAt)}
                    </div>
                  </div>
                ) : (
                  <Text type="secondary">尚未绑定，点击绑定后将跳转到 {item.displayName} 授权页。</Text>
                )
              }
            />
          </List.Item>
        )}
      />
    </Card>
  );
}
