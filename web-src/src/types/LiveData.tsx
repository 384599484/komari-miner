export type LiveData = {
    online: string[];
    data: { [key: string]: Record };
};

export type Record = {
  cpu: {
    usage: number;
  };
  ram: {
    used: number;
  };
  swap: {
    used: number;
  };
  load: {
    load1: number;
    load5: number;
    load15: number;
  };
  disk: {
    used: number;
  };
  network: {
    up: number;
    down: number;
    totalUp: number;
    totalDown: number;
  };
  connections: {
    tcp: number;
    udp: number;
  };
  gpu?: {
    count: number;
    average_usage: number;
    detailed_info: {
      index?: number;
      name: string;
      memory_total: number;
      memory_used: number;
      utilization: number;
      temperature: number;
      memory_temperature?: number;
      power?: number;
      power_limit?: number;
      fan_speed?: number;
      clock_mhz?: number;
      memory_clock_mhz?: number;
    }[];
  };
  mining?: MiningReport;
  uptime: number;
  process: number;
  message: string;
  updated_at: string;
};

export type MiningMiner = {
  pid: number;
  name: string;
  user?: string;
  algorithm?: string;
  hashrate?: number;
  hashrate_text?: string;
  accepted_shares?: number;
  rejected_shares?: number;
  pool?: string;
  wallet?: string;
  gpus?: number[];
  cpu_percent?: number;
  memory_bytes?: number;
  uptime?: number;
  command?: string;
  exe?: string;
  status?: string;
};

export type MiningReport = {
  miner_count: number;
  total_hashrate: number;
  total_power?: number;
  miners: MiningMiner[];
  alerts?: string[];
};

export type LiveDataResponse = {
  data: LiveData;
  status: string;
};
