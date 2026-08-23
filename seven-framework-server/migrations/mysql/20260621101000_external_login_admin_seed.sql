-- +goose Up
SET @operatorId := 0;
SET @externalLoginPermissionBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 1900301200) FROM sys_permission);

UPDATE sys_permission existing
JOIN (
  SELECT 1 AS sortOrder, 'system:external-login-provider:list' AS code, '外部登录提供方列表' AS name, 'GET' AS method, '/external-login/admin/providers' AS path, '分页查询外部登录提供方列表' AS description
  UNION ALL SELECT 2, 'system:external-login-provider:query', '外部登录提供方详情', 'GET', '/external-login/admin/providers/:providerCode', '查询外部登录提供方详情'
  UNION ALL SELECT 3, 'system:external-login-provider:add', '创建外部登录提供方', 'POST', '/external-login/admin/providers', '创建外部登录提供方'
  UNION ALL SELECT 4, 'system:external-login-provider:edit', '编辑外部登录提供方', 'PUT', '/external-login/admin/providers/:providerCode', '编辑外部登录提供方'
  UNION ALL SELECT 5, 'system:external-login-provider:status', '启停外部登录提供方', 'PUT', '/external-login/admin/providers/:providerCode/status', '启用或停用外部登录提供方'
  UNION ALL SELECT 6, 'system:external-login-provider:secret:rotate', '轮换外部登录密钥', 'POST', '/external-login/admin/providers/:providerCode/client-secret/rotate', '轮换外部登录提供方 client secret'
  UNION ALL SELECT 7, 'system:external-login-identity:list', '外部身份绑定列表', 'GET', '/external-login/admin/identities', '分页查询外部身份绑定列表'
  UNION ALL SELECT 8, 'system:external-login-identity:status', '变更外部身份状态', 'PUT', '/external-login/admin/identities/:identityId/status', '启用、停用或解绑外部身份'
  UNION ALL SELECT 9, 'system:external-oauth-token:list', '外部 OAuth Token 列表', 'GET', '/external-login/admin/tokens', '分页查询外部 OAuth token 列表'
  UNION ALL SELECT 10, 'system:external-oauth-token:revoke', '撤销外部 OAuth Token', 'PUT', '/external-login/admin/tokens/:tokenId/revoke', '撤销外部 OAuth token'
) item ON existing.code = item.code
SET existing.name = item.name,
    existing.resourceType = 'API',
    existing.method = item.method,
    existing.path = item.path,
    existing.status = 0,
    existing.description = item.description,
    existing.updateTime = NOW(),
    existing.isDeleted = 0;

INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT @externalLoginPermissionBaseId + ROW_NUMBER() OVER (ORDER BY item.sortOrder), item.code, item.name, 'API', item.method, item.path, 0, item.description, NOW(), NOW(), 0
FROM (
  SELECT 1 AS sortOrder, 'system:external-login-provider:list' AS code, '外部登录提供方列表' AS name, 'GET' AS method, '/external-login/admin/providers' AS path, '分页查询外部登录提供方列表' AS description
  UNION ALL SELECT 2, 'system:external-login-provider:query', '外部登录提供方详情', 'GET', '/external-login/admin/providers/:providerCode', '查询外部登录提供方详情'
  UNION ALL SELECT 3, 'system:external-login-provider:add', '创建外部登录提供方', 'POST', '/external-login/admin/providers', '创建外部登录提供方'
  UNION ALL SELECT 4, 'system:external-login-provider:edit', '编辑外部登录提供方', 'PUT', '/external-login/admin/providers/:providerCode', '编辑外部登录提供方'
  UNION ALL SELECT 5, 'system:external-login-provider:status', '启停外部登录提供方', 'PUT', '/external-login/admin/providers/:providerCode/status', '启用或停用外部登录提供方'
  UNION ALL SELECT 6, 'system:external-login-provider:secret:rotate', '轮换外部登录密钥', 'POST', '/external-login/admin/providers/:providerCode/client-secret/rotate', '轮换外部登录提供方 client secret'
  UNION ALL SELECT 7, 'system:external-login-identity:list', '外部身份绑定列表', 'GET', '/external-login/admin/identities', '分页查询外部身份绑定列表'
  UNION ALL SELECT 8, 'system:external-login-identity:status', '变更外部身份状态', 'PUT', '/external-login/admin/identities/:identityId/status', '启用、停用或解绑外部身份'
  UNION ALL SELECT 9, 'system:external-oauth-token:list', '外部 OAuth Token 列表', 'GET', '/external-login/admin/tokens', '分页查询外部 OAuth token 列表'
  UNION ALL SELECT 10, 'system:external-oauth-token:revoke', '撤销外部 OAuth Token', 'PUT', '/external-login/admin/tokens/:tokenId/revoke', '撤销外部 OAuth token'
) item
WHERE NOT EXISTS (SELECT 1 FROM sys_permission existing WHERE existing.code = item.code);

SET @rootMenuId := (
  SELECT id
  FROM sys_menu
  WHERE path = '/system' AND isDeleted = 0
  ORDER BY visible DESC, sortOrder ASC, id
  LIMIT 1
);
SET @accessMenuId := (SELECT id FROM sys_menu WHERE path = '/system/access' AND isDeleted = 0 ORDER BY id LIMIT 1);
SET @externalLoginMenuBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 2012233000000000000) FROM sys_menu);

INSERT INTO sys_menu (id, name, parentId, sortOrder, path, component, icon, type, permission, isFrame, isCache, visible, hierarchy, level, status, creatorId, createTime, updaterId, updateTime, isDeleted, remark)
SELECT @externalLoginMenuBaseId + 1, '外部登录', @accessMenuId, 50, '/system/external-login-provider', 'system/external-login-provider/index', 'SafetyOutlined', 'C', 'system:external-login-provider:list', 1, 1, 1, CONCAT('/1/', @rootMenuId, '/', @accessMenuId, '/', @externalLoginMenuBaseId + 1), 3, 0, @operatorId, NOW(), @operatorId, NOW(), 0, '外部登录提供方管理'
WHERE @accessMenuId IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM sys_menu existing WHERE existing.path = '/system/external-login-provider' AND existing.isDeleted = 0);

UPDATE sys_menu child
JOIN sys_menu parent ON parent.path = '/system/access' AND parent.isDeleted = 0
LEFT JOIN sys_menu root ON root.path = '/system' AND root.isDeleted = 0
SET child.name = '外部登录',
    child.parentId = parent.id,
    child.sortOrder = 50,
    child.component = 'system/external-login-provider/index',
    child.icon = 'SafetyOutlined',
    child.type = 'C',
    child.permission = 'system:external-login-provider:list',
    child.visible = 1,
    child.status = 0,
    child.hierarchy = CONCAT('/1/', COALESCE(root.id, parent.parentId), '/', parent.id, '/', child.id),
    child.level = 3,
    child.updateTime = NOW()
WHERE child.path = '/system/external-login-provider' AND child.isDeleted = 0;

SET @externalLoginMenuId := (SELECT id FROM sys_menu WHERE path = '/system/external-login-provider' AND isDeleted = 0 ORDER BY id LIMIT 1);
SET @menuPermissionBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 2012233000000000100) FROM sys_menu_permission);

INSERT INTO sys_menu_permission (id, menuId, permissionId, creatorId, createTime)
SELECT @menuPermissionBaseId + ROW_NUMBER() OVER (ORDER BY p.id), @externalLoginMenuId, p.id, @operatorId, NOW()
FROM sys_permission p
WHERE p.code IN (
  'system:external-login-provider:list',
  'system:external-login-provider:query',
  'system:external-login-provider:add',
  'system:external-login-provider:edit',
  'system:external-login-provider:status',
  'system:external-login-provider:secret:rotate',
  'system:external-login-identity:list',
  'system:external-login-identity:status',
  'system:external-oauth-token:list',
  'system:external-oauth-token:revoke'
)
AND @externalLoginMenuId IS NOT NULL
AND NOT EXISTS (
  SELECT 1 FROM sys_menu_permission existing WHERE existing.menuId = @externalLoginMenuId AND existing.permissionId = p.id
);

SET @roleMenuBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 2012233000000000200) FROM sys_role_menu);

INSERT INTO sys_role_menu (id, roleId, menuId, createTime, updateTime)
SELECT @roleMenuBaseId + ROW_NUMBER() OVER (ORDER BY candidate.roleId, candidate.menuId), candidate.roleId, candidate.menuId, NOW(), NOW()
FROM (
  SELECT r.id AS roleId, m.id AS menuId
  FROM sys_role r
  JOIN sys_menu m ON m.path IN ('/system/access', '/system/external-login-provider') AND m.isDeleted = 0
  WHERE r.code = 'SUPER_ADMIN' AND r.isDeleted = 0
    AND NOT EXISTS (SELECT 1 FROM sys_role_menu existing WHERE existing.roleId = r.id AND existing.menuId = m.id)
) candidate;

SET @rolePermissionBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 2012233000000000300) FROM sys_role_permission);

INSERT INTO sys_role_permission (id, roleId, permissionId, creatorId, createTime, updateTime)
SELECT @rolePermissionBaseId + ROW_NUMBER() OVER (ORDER BY candidate.roleId, candidate.permissionId), candidate.roleId, candidate.permissionId, @operatorId, NOW(), NOW()
FROM (
  SELECT r.id AS roleId, p.id AS permissionId
  FROM sys_role r
  JOIN sys_permission p ON p.isDeleted = 0 AND p.code IN (
    'system:external-login-provider:list',
    'system:external-login-provider:query',
    'system:external-login-provider:add',
    'system:external-login-provider:edit',
    'system:external-login-provider:status',
    'system:external-login-provider:secret:rotate',
    'system:external-login-identity:list',
    'system:external-login-identity:status',
    'system:external-oauth-token:list',
    'system:external-oauth-token:revoke'
  )
  WHERE r.code = 'SUPER_ADMIN' AND r.isDeleted = 0
    AND NOT EXISTS (SELECT 1 FROM sys_role_permission existing WHERE existing.roleId = r.id AND existing.permissionId = p.id)
) candidate;

-- +goose Down
-- Stable seed data is intentionally retained on rollback to avoid deleting
-- existing security menus, role bindings, or later operator-maintained grants.
SELECT 1;
