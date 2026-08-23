import type { ReactNode } from 'react';
import {
  ApartmentOutlined,
  AppstoreOutlined,
  BankOutlined,
  ClusterOutlined,
  CrownFilled,
  DeploymentUnitOutlined,
  FileTextOutlined,
  GlobalOutlined,
  IdcardOutlined,
  MenuOutlined,
  NotificationOutlined,
  RadarChartOutlined,
  SafetyOutlined,
  SettingOutlined,
  TeamOutlined,
  UserOutlined,
} from '@ant-design/icons';
import type { AppRoute } from 'config/route';
import type { RuntimeFeatures } from '@/lib/http/types';
import { NOTIFICATION_PERMISSIONS } from '@/lib/auth/permissionCodes';
import { filterRoutesByRuntimeFeatures } from '@/lib/navigation/runtimeFeatureGate';

const iconMap: Record<string, ReactNode> = {
  ApartmentOutlined: <ApartmentOutlined />,
  AppstoreOutlined: <AppstoreOutlined />,
  BankOutlined: <BankOutlined />,
  ClusterOutlined: <ClusterOutlined />,
  CrownFilled: <CrownFilled />,
  DeploymentUnitOutlined: <DeploymentUnitOutlined />,
  FileTextOutlined: <FileTextOutlined />,
  GlobalOutlined: <GlobalOutlined />,
  IdcardOutlined: <IdcardOutlined />,
  MenuOutlined: <MenuOutlined />,
  NotificationOutlined: <NotificationOutlined />,
  RadarChartOutlined: <RadarChartOutlined />,
  SafetyOutlined: <SafetyOutlined />,
  SettingOutlined: <SettingOutlined />,
  TeamOutlined: <TeamOutlined />,
  UserOutlined: <UserOutlined />,
};

function normalizeIconName(icon?: string) {
  return (icon ?? '').trim().replace(/^<|>$/g, '');
}

function resolveIcon(icon?: string) {
  const iconName = normalizeIconName(icon);
  if (!iconName) {
    return undefined;
  }
  return iconMap[iconName] ?? iconMap[`${iconName}Outlined`] ?? <AppstoreOutlined />;
}

function normalizePath(path?: string) {
  const value = (path ?? '').trim();
  if (!value) {
    return '';
  }
  if (/^https?:\/\//i.test(value)) {
    return value;
  }
  return value.startsWith('/') ? value : `/${value}`;
}

type MenuRouteSource = API.MenuTreeVO | API.MenuVO;

function menuSortValue(item: MenuRouteSource) {
  return item.sortOrder ?? Number.MAX_SAFE_INTEGER;
}

function isRenderableMenu(item: MenuRouteSource) {
  const type = (item.type ?? '').trim().toUpperCase();
  if (type === 'F') {
    return false;
  }
  return item.status === undefined || item.status === 0;
}

function toRoute(item: MenuRouteSource, parentKey: string): AppRoute | null {
  if (!isRenderableMenu(item)) {
    return null;
  }

  const children = [...(item.children ?? [])]
    .sort((left, right) => menuSortValue(left) - menuSortValue(right))
    .map((child) => toRoute(child, `${parentKey}-${item.id ?? item.path ?? item.name ?? 'menu'}`))
    .filter((child): child is AppRoute => Boolean(child));

  const type = (item.type ?? '').trim().toUpperCase();
  if (type === 'M' && children.length === 0) {
    return null;
  }

  const path = normalizePath(item.path);
  if (!path && children.length === 0) {
    return null;
  }

  const permission = (item.permission ?? '').trim();
  const featureCode = (item.featureCode ?? '').trim();
  const requiredPermissions =
    path === '/system/notification'
      ? [
          NOTIFICATION_PERMISSIONS.CHANNEL_LIST,
          NOTIFICATION_PERMISSIONS.TEMPLATE_LIST,
          NOTIFICATION_PERMISSIONS.SCENE_LIST,
          NOTIFICATION_PERMISSIONS.DELIVERY_LIST,
        ]
      : permission
        ? [permission]
        : undefined;
  const route: AppRoute = {
    key: `db-menu-${item.id ?? path ?? parentKey}`,
    path: path || `/${parentKey}-${item.id ?? item.name ?? 'group'}`,
    name: item.name || path || '未命名菜单',
    icon: resolveIcon(item.icon),
    component: item.component,
    hideInMenu: item.visible === 0,
    requiredPermissions,
    permissionMatchMode: path === '/system/notification' ? 'any' : undefined,
    featureCode: featureCode || undefined,
    routes: children.length > 0 ? children : undefined,
  };

  return route;
}

export function buildRoutesFromMenuTree(
  menuTree?: ReadonlyArray<MenuRouteSource> | null,
  runtimeFeatures?: RuntimeFeatures | null,
): AppRoute[] {
  if (!Array.isArray(menuTree) || menuTree.length === 0) {
    return [];
  }
  const routes = [...menuTree]
    .sort((left, right) => menuSortValue(left) - menuSortValue(right))
    .map((item) => toRoute(item, 'root'))
    .filter((item): item is AppRoute => Boolean(item));
  return filterRoutesByRuntimeFeatures(routes, runtimeFeatures);
}

export function getStaticSupportRoutes(
  routes: AppRoute[],
  runtimeFeatures?: RuntimeFeatures | null,
) {
  return filterRoutesByRuntimeFeatures(
    routes.filter((route) => route.hideInMenu || route.layout === false),
    runtimeFeatures,
  );
}
