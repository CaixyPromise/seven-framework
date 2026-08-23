export interface SysOrg {
  id?: number;
  isDeleted?: number;
  createTime?: string;
  updateTime?: string;
  code?: string;
  name?: string;
  parentId?: number;
  status?: number;
  sortOrder?: number;
  leaderUserId?: number;
  children?: SysOrg[];
  parentName?: string;
}
