export interface PageResult<T> {
  records: T[];
  total: number;
  size: number;
  current: number;
}

export interface UserRecord {
  id: number;
  userId?: number;
  nickName?: string;
  userName?: string;
  userPhone?: string;
  userEmail?: string;
  userGender?: number;
  userRole?: string;
  status?: number;
  unsealTime?: string;
  createTime?: string;
  updateTime?: string;
}

export interface OnlineUserRecord {
  userId?: number;
  id?: number;
  username?: string;
  nickname?: string;
  userAccount?: string;
  userRole?: string;
  loginTime?: number;
  lastActiveTime?: number;
  loginIp?: string;
  loginAddress?: string;
  browser?: string;
  os?: string;
  deviceId?: string;
  isCurrentSession?: boolean;
}

export interface OperationLogRecord {
  id: number;
  username?: string;
  requestUrl?: string;
  requestMethod?: string;
  operationType?: string;
  status?: number;
  executionTime?: number;
  startTime?: string;
  endTime?: string;
  createTime?: string;
}

export interface ApiResponse<T> {
  code: number;
  message?: string;
  data: T;
}

export interface BatchLogoutResult {
  successCount?: number;
  failedCount?: number;
  successIds?: number[];
  failedIds?: number[];
  failureReasons?: string[];
}
