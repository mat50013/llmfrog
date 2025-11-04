import { useEffect, useState } from "react";
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from "../components";
import { Activity } from "lucide-react";
import { motion } from "framer-motion";

interface GPUDevice {
  index: number;
  name: string;
  uuid?: string;
  memoryTotal: number;
  memoryFree: number;
  memoryUsed: number;
  utilization?: number;
  temperature?: number;
  powerDraw?: number;
  powerLimit?: number;
  driver?: string;
}

interface GPUStats {
  gpus: GPUDevice[];
  totalGPUs: number;
  totalMemory: number;
  totalFree: number;
  backend: string;
  systemRAM?: {
    total: number;
    free: number;
    used: number;
    usagePercent: number;
  };
}

const formatMemory = (gb: number): string => {
  return gb.toFixed(2) + " GB";
};

const formatPercent = (value: number): string => {
  return value.toFixed(1) + "%";
};

const GPUMonitor = () => {
  const [gpuStats, setGPUStats] = useState<GPUStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isUpdating, setIsUpdating] = useState(false);

  const fetchGPUStats = async () => {
    setIsUpdating(true);
    try {
      const response = await fetch('/api/gpu/stats');
      if (!response.ok) {
        throw new Error('Failed to fetch GPU stats');
      }
      const data = await response.json();
      setGPUStats(data);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error');
    } finally {
      setLoading(false);
      setTimeout(() => setIsUpdating(false), 300); // Brief flash for update indicator
    }
  };

  useEffect(() => {
    fetchGPUStats();
    const interval = setInterval(fetchGPUStats, 2000); // Update every 2 seconds
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <Activity className="w-8 h-8 animate-spin text-brand-500" />
      </div>
    );
  }

  if (error) {
    return (
      <Card>
        <CardContent>
          <div className="text-red-500">Error: {error}</div>
        </CardContent>
      </Card>
    );
  }

  if (!gpuStats) {
    return (
      <Card>
        <CardContent>
          <div>No GPU data available</div>
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-6">
      {/* Summary Card */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle className="flex items-center gap-2">
                <span className="text-lg">🐸</span>
                Frog Pond GPU Status
              </CardTitle>
              <CardDescription>
                🌊 Pond Backend: {gpuStats.backend} | Lily Pads: {gpuStats.totalGPUs}
              </CardDescription>
            </div>
            <div className="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
              <motion.div
                animate={{ opacity: isUpdating ? 1 : 0.5 }}
                className="w-2 h-2 rounded-full bg-green-500"
              />
              <span>Live</span>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div className="bg-background-secondary p-4 rounded-lg">
              <div className="text-sm text-gray-700 dark:text-gray-300 mb-1 font-medium">🌊 Total Pond Water</div>
              <div className="text-2xl font-bold text-gray-900 dark:text-brand-500">
                {formatMemory(gpuStats.totalMemory)}
              </div>
            </div>
            <div className="bg-background-secondary p-4 rounded-lg">
              <div className="text-sm text-gray-700 dark:text-gray-300 mb-1 font-medium">💎 Crystal Clear</div>
              <div className="text-2xl font-bold text-blue-600 dark:text-blue-400">
                {formatMemory(gpuStats.totalFree)}
              </div>
            </div>
            <div className="bg-background-secondary p-4 rounded-lg">
              <div className="text-sm text-gray-700 dark:text-gray-300 mb-1 font-medium">🐸 Frog Occupied</div>
              <div className="text-2xl font-bold text-orange-600 dark:text-orange-500">
                {formatMemory(gpuStats.totalMemory - gpuStats.totalFree)}
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Individual GPU Cards */}
      {gpuStats.gpus.length > 0 ? (
        gpuStats.gpus.map((gpu, index) => (
          <motion.div
            key={gpu.index}
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: index * 0.1 }}
          >
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <span className="text-lg">🐸</span>
                  Lily Pad {gpu.index}: {gpu.name}
                </CardTitle>
                {gpu.driver && (
                  <CardDescription>Driver: {gpu.driver}</CardDescription>
                )}
              </CardHeader>
              <CardContent>
                <div className="space-y-4">
                  {/* Memory Usage Bar */}
                  <div>
                    <div className="flex justify-between text-sm mb-2">
                      <span className="text-gray-700 dark:text-gray-300 font-medium">🌊 Water Usage</span>
                      <span className="font-medium">
                        {formatMemory(gpu.memoryUsed)} / {formatMemory(gpu.memoryTotal)}
                      </span>
                    </div>
                    <div
                      className="w-full bg-background-secondary rounded-full h-3 overflow-hidden"
                      role="progressbar"
                      aria-label={`GPU ${gpu.index} memory usage`}
                      aria-valuenow={Math.round((gpu.memoryUsed / gpu.memoryTotal) * 100)}
                      aria-valuemin={0}
                      aria-valuemax={100}
                    >
                      <motion.div
                        className="h-full bg-gradient-to-r from-brand-500 to-brand-600"
                        initial={{ width: 0 }}
                        animate={{
                          width: `${(gpu.memoryUsed / gpu.memoryTotal) * 100}%`
                        }}
                        transition={{ duration: 0.5, ease: "easeOut" }}
                      />
                    </div>
                  </div>

                  {/* GPU Stats Grid */}
                  <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                    {gpu.utilization !== undefined && (
                      <div className="bg-background-secondary p-3 rounded-lg">
                        <div className="text-xs text-gray-700 dark:text-gray-300 mb-1">Utilization</div>
                        <div className="text-lg font-bold text-blue-600 dark:text-blue-400">
                          {formatPercent(gpu.utilization)}
                        </div>
                      </div>
                    )}
                    {gpu.temperature !== undefined && (
                      <div className="bg-background-secondary p-3 rounded-lg">
                        <div className="text-xs text-gray-700 dark:text-gray-300 mb-1">Temperature</div>
                        <div className={`text-lg font-semibold ${
                          gpu.temperature > 80 ? 'text-red-400' :
                          gpu.temperature > 60 ? 'text-yellow-400' : 'text-blue-400'
                        }`}>
                          {gpu.temperature}°C
                        </div>
                      </div>
                    )}
                    {gpu.powerDraw !== undefined && (
                      <div className="bg-background-secondary p-3 rounded-lg">
                        <div className="text-xs text-gray-700 dark:text-gray-300 mb-1">Power Draw</div>
                        <div className="text-lg font-semibold text-blue-400">
                          {gpu.powerDraw.toFixed(1)}W
                        </div>
                      </div>
                    )}
                    <div className="bg-background-secondary p-3 rounded-lg">
                      <div className="text-xs text-gray-700 dark:text-gray-300 mb-1">Free Memory</div>
                      <div className="text-lg font-semibold text-blue-400">
                        {formatMemory(gpu.memoryFree)}
                      </div>
                    </div>
                  </div>
                </div>
              </CardContent>
            </Card>
          </motion.div>
        ))
      ) : (
        <Card>
          <CardContent>
            <div className="text-center py-8 text-gray-600 dark:text-gray-300">
              🐸 No lily pads found in the pond. Frogs are hopping on CPU land! 🏞️
            </div>
          </CardContent>
        </Card>
      )}

      {/* System RAM Card */}
      {gpuStats.systemRAM && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <span className="text-lg">🏞️</span>
              Frog Habitat (System RAM)
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              <div>
                <div className="flex justify-between text-sm mb-2">
                  <span className="text-gray-700 dark:text-gray-300">RAM Usage</span>
                  <span className="font-medium">
                    {formatMemory(gpuStats.systemRAM.used)} / {formatMemory(gpuStats.systemRAM.total)}
                    {" "}({formatPercent(gpuStats.systemRAM.usagePercent)})
                  </span>
                </div>
                <div className="w-full bg-background-secondary rounded-full h-3 overflow-hidden">
                  <motion.div
                    className="h-full bg-gradient-to-r from-brand-500 to-brand-600"
                    initial={{ width: 0 }}
                    animate={{
                      width: `${gpuStats.systemRAM.usagePercent}%`
                    }}
                    transition={{ duration: 0.5, ease: "easeOut" }}
                  />
                </div>
              </div>
              <div className="grid grid-cols-3 gap-4">
                <div className="bg-background-secondary p-3 rounded-lg">
                  <div className="text-xs text-gray-700 dark:text-gray-300 mb-1">Total</div>
                  <div className="text-lg font-semibold text-blue-400">{formatMemory(gpuStats.systemRAM.total)}</div>
                </div>
                <div className="bg-background-secondary p-3 rounded-lg">
                  <div className="text-xs text-gray-700 dark:text-gray-300 mb-1">Used</div>
                  <div className="text-lg font-semibold text-orange-400">{formatMemory(gpuStats.systemRAM.used)}</div>
                </div>
                <div className="bg-background-secondary p-3 rounded-lg">
                  <div className="text-xs text-gray-700 dark:text-gray-300 mb-1">Free</div>
                  <div className="text-lg font-semibold text-green-400">{formatMemory(gpuStats.systemRAM.free)}</div>
                </div>
              </div>
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
};

export default GPUMonitor;