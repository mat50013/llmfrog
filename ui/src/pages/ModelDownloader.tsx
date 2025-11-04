import React, { useState, useEffect } from 'react';
import { motion } from 'framer-motion';
import {
  SearchIcon,
  DownloadIcon,
  FilterIcon,
  SettingsIcon,
  KeyIcon,
  DatabaseIcon,
  CpuIcon,
  HardDriveIcon,
  StarIcon,
  ClockIcon,
  ExternalLinkIcon,
  AlertCircleIcon,
  XIcon
} from 'lucide-react';
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { Input } from '../components/ui/Input';
import { Modal } from '../components/ui/Modal';
import DownloadDestinationModal from '../components/DownloadDestinationModal';

// Types
interface HFModel {
  id: string;
  author: string;
  downloads: number;
  likes: number;
  updatedAt: string;
  lastModified?: string;
  tags: string[];
  pipeline_tag?: string;
  siblings?: Array<{
    rfilename: string;
    size?: number;
  }>;
  gguf?: {
    total?: number;
    architecture?: string;
    context_length?: number;
    bos_token?: string;
    eos_token?: string;
  };
  cardData?: {
    license?: string;
    base_model?: string[];
  };
}

interface GPUInfo {
  index: number;
  name: string;
  memoryTotal: number;
  memoryFree: number;
  memoryUsed: number;
  utilization?: number;
  temperature?: number;
  powerDraw?: number;
  powerLimit?: number;
}

interface SystemSpecs {
  totalRAM: number;
  availableRAM: number;
  totalVRAM: number;
  availableVRAM: number;
  cpuCores: number;
  gpuName: string;
  gpus?: GPUInfo[];  // Array of individual GPUs
  diskSpace: number;
}

interface DownloadProgress {
  id: string;
  modelId: string;
  filename: string;
  url: string;
  filePath?: string; // Full path to the downloaded file
  progress: number;
  speed: number;
  bytesDownloaded: number;
  totalBytes: number;
  estimatedTimeRemaining: number;
  status: 'pending' | 'downloading' | 'paused' | 'completed' | 'failed' | 'cancelled';
  error?: string;
  startTime: string;
  endTime?: string;
  retryCount: number;
}

const ModelDownloaderPage: React.FC = () => {
  // State
  const [searchQuery, setSearchQuery] = useState('');
  const [hfApiKey, setHfApiKey] = useState('');
  const [models, setModels] = useState<HFModel[]>([]);
  const [loading, setLoading] = useState(false);
  const [selectedModel, setSelectedModel] = useState<HFModel | null>(null);
  const [loadingModelDetails, setLoadingModelDetails] = useState(false);
  const [downloads, setDownloads] = useState<DownloadProgress[]>([]);
  const [systemSpecs, setSystemSpecs] = useState<SystemSpecs | null>(null);
  const [showSettings, setShowSettings] = useState(false);
  const [selectedFilter, setSelectedFilter] = useState<string>('all');
  const [showDestinationModal, setShowDestinationModal] = useState(false);
  const [pendingDownload, setPendingDownload] = useState<{
    model: HFModel; 
    filename: string; 
    isMultiPart?: boolean;
    parts?: Array<{ path: string; size: number }>;
  } | null>(null);

  // Filters
  const filters = [
    { key: 'all', label: 'All Models', count: models.length },
    { key: 'text-generation', label: 'Text Generation', count: models.filter(m => m.pipeline_tag === 'text-generation').length },
    { key: 'conversational', label: 'Chat Models', count: models.filter(m => m.pipeline_tag === 'conversational').length },
    { key: 'code', label: 'Code Models', count: models.filter(m => m.tags?.includes('code')).length },
  ];

  // Filter models
  const filteredModels = models.filter(model => {
    if (selectedFilter === 'all') return true;
    if (selectedFilter === 'code') return model.tags?.includes('code');
    return model.pipeline_tag === selectedFilter;
  });

  // Fetch system specs and GPU info
  useEffect(() => {
    const fetchSystemSpecs = async () => {
      try {
        // Fetch basic system specs
        const specsResponse = await fetch('/api/system/specs');
        let specs: SystemSpecs | null = null;
        if (specsResponse.ok) {
          specs = await specsResponse.json();
        }

        // Also fetch detailed GPU stats
        const gpuResponse = await fetch('/api/gpu/stats');
        if (gpuResponse.ok) {
          const gpuData = await gpuResponse.json();
          if (specs && gpuData.gpus && gpuData.gpus.length > 0) {
            // Enhance specs with detailed GPU information
            specs.gpus = gpuData.gpus.map((gpu: any) => ({
              index: gpu.index || 0,
              name: gpu.name || 'Unknown GPU',
              memoryTotal: gpu.memoryTotal || 0,
              memoryFree: gpu.memoryFree || 0,
              memoryUsed: gpu.memoryUsed || 0,
              utilization: gpu.utilization,
              temperature: gpu.temperature,
              powerDraw: gpu.powerDraw,
              powerLimit: gpu.powerLimit
            }));
          }
        }

        if (specs) {
          setSystemSpecs(specs);
        }
      } catch (error) {
        console.error('Failed to fetch system specs:', error);
      }
    };
    fetchSystemSpecs();
  }, []);

  // Load API key
  useEffect(() => {
    const loadApiKey = async () => {
      try {
        const response = await fetch('/api/settings/hf-api-key');
        if (response.ok) {
          const data = await response.json();
          setHfApiKey(data.apiKey || '');
        }
      } catch (error) {
        console.error('Failed to load API key:', error);
      }
    };
    loadApiKey();
  }, []);

  // Fetch current downloads
  useEffect(() => {
    const fetchDownloads = async () => {
      try {
        const response = await fetch('/api/models/downloads');
        if (response.ok) {
          const downloads = await response.json();
          const downloadArray = Object.values(downloads) as DownloadProgress[];
          setDownloads(downloadArray);
        }
      } catch (error) {
        console.error('Failed to fetch downloads:', error);
      }
    };
    fetchDownloads();
  }, []);

  // Set up real-time download progress updates
  useEffect(() => {
    // Include API key via query param for SSE
    let url = '/api/events';
    try {
      const k = localStorage.getItem('cc_api_key');
      if (k && k.trim()) {
        const qp = new URLSearchParams({ api_key: k.trim() });
        url = `/api/events?${qp.toString()}`;
      }
    } catch {}
    const eventSource = new EventSource(url);
    
    eventSource.onmessage = (event) => {
      try {
        const message = JSON.parse(event.data);
        if (message.type === 'downloadProgress') {
          const progressData = JSON.parse(message.data);
          
          setDownloads(prev => {
            const updated = [...prev];
            const index = updated.findIndex(d => d.id === progressData.downloadId);
            
            if (index !== -1) {
              const oldStatus = updated[index].status;
              const newStatus = progressData.info?.status;
              
              // Update existing download
              updated[index] = { ...updated[index], ...progressData.info };
              
              // Check if download just completed
              if (oldStatus !== 'completed' && newStatus === 'completed') {
                const completedDownload = updated[index];
                if (completedDownload.filename && completedDownload.filename.toLowerCase().endsWith('.gguf')) {
                  // Use the actual file path from the download info (respects custom destination)
                  const filePath = completedDownload.filePath || `./downloads/${completedDownload.filename}`;
                  
                  // Add model to config after a short delay to ensure file is fully written
                  setTimeout(() => {
                    addModelToConfig(filePath);
                  }, 2000);
                }
              }
            } else if (progressData.info) {
              // Add new download
              updated.push(progressData.info);
            }
            
            return updated;
          });
        }
      } catch (error) {
        console.error('Error parsing download progress event:', error);
      }
    };

    return () => {
      eventSource.close();
    };
  }, []);

  // Fetch detailed model information when a model is selected
  useEffect(() => {
    const fetchModelDetails = async () => {
      if (!selectedModel) return;
      
      setLoadingModelDetails(true);
      try {
        const headers: Record<string, string> = {
          'Content-Type': 'application/json',
        };
        
        if (hfApiKey) {
          headers['Authorization'] = `Bearer ${hfApiKey}`;
        }

        // Fetch full model details from HuggingFace API
        const response = await fetch(
          `https://huggingface.co/api/models/${selectedModel.id}`,
          { headers }
        );

        if (response.ok) {
          const detailedModel = await response.json();
          
          // Process and group split models from the original siblings data
          const originalSiblings = detailedModel.siblings || [];
          let siblingsWithSizes = originalSiblings;

          // Get GGUF files that need size information
          const ggufFiles = originalSiblings.filter((s: any) =>
            s.rfilename.toLowerCase().endsWith('.gguf')
          );

          // Always process siblings to group split models, even without size info
          if (ggufFiles.length > 0) {
            // Helper function to detect split model patterns
            const isSplitModel = (filename: string): boolean => {
              const splitPatterns = [
                /-\d{5}-of-\d{5}\.gguf$/i,
                /\.gguf\.part\d+of\d+$/i,
                /-part-\d{5}-of-\d{5}\.gguf$/i,
                /\.split-\d{5}-of-\d{5}\.gguf$/i
              ];
              return splitPatterns.some(pattern => pattern.test(filename));
            };

            // Helper to extract base name from split model
            const getBaseName = (filename: string): string => {
              return filename
                .replace(/-\d{5}-of-\d{5}\.gguf$/i, '')
                .replace(/\.gguf\.part\d+of\d+$/i, '')
                .replace(/-part-\d{5}-of-\d{5}\.gguf$/i, '')
                .replace(/\.split-\d{5}-of-\d{5}\.gguf$/i, '');
            };

            // Group split models from the original siblings data
            const splitModelGroups = new Map<string, any[]>();
            const singleFiles: any[] = [];

            ggufFiles.forEach((file: any) => {
              const filename = file.rfilename.split('/').pop() || file.rfilename;
              if (isSplitModel(filename)) {
                const baseName = getBaseName(filename);
                if (!splitModelGroups.has(baseName)) {
                  splitModelGroups.set(baseName, []);
                }
                splitModelGroups.get(baseName)!.push(file);
              } else {
                singleFiles.push({
                  rfilename: file.rfilename,
                  size: file.size || 0,
                  isMultiPart: false
                });
              }
            });

            // Create processed files array with grouped split models
            const processedFiles: any[] = [];

            // Add grouped split models
            splitModelGroups.forEach((parts, baseName) => {
              parts.sort((a, b) => a.rfilename.localeCompare(b.rfilename));
              const totalSize = parts.reduce((sum, f) => sum + (f.size || 0), 0);

              let displayName = baseName;
              const quantMatch = baseName.match(/\.(Q\d+_[A-Z_]+|FP16|BF16|IQ\d+_[A-Z]+)$/i);
              if (quantMatch) {
                displayName = quantMatch[1];
              } else if (baseName.includes('.')) {
                displayName = baseName.split('.').pop() || baseName;
              }

              processedFiles.push({
                rfilename: `${displayName} (Split Model)`,
                size: totalSize,
                isMultiPart: true,
                parts: parts.map(f => ({ path: f.rfilename, size: f.size || 0 })),
                partCount: parts.length
              });
            });

            // Add single files
            processedFiles.push(...singleFiles);

            // Use processed files as the initial state
            siblingsWithSizes = processedFiles;
          }

          // Try to enhance with detailed size information from tree API
          if (ggufFiles.length > 0) {
            try {
              // Fetch the full repository tree which includes file sizes
              const treeResponse = await fetch(
                `https://huggingface.co/api/models/${selectedModel.id}/tree/main`,
                { headers }
              );

              if (treeResponse.ok) {
                const treeData = await treeResponse.json();

                // Create a map of file paths to sizes from tree data
                const fileSizeMap = new Map<string, number>();

                // Process tree data to get file sizes
                const processTreeData = (items: any[]) => {
                  items.forEach((item: any) => {
                    if (item.type === 'file' && item.path.toLowerCase().endsWith('.gguf') && item.size) {
                      fileSizeMap.set(item.path, item.size);
                    }
                  });
                };

                processTreeData(treeData);

                // Also fetch directory contents for more complete size information
                const directoriesToFetch = treeData
                  .filter((item: any) => item.type === 'directory')
                  .map((item: any) => item.path);

                const directoryPromises = directoriesToFetch.map(async (dirPath: string) => {
                  try {
                    const dirResponse = await fetch(
                      `https://huggingface.co/api/models/${selectedModel.id}/tree/main/${encodeURIComponent(dirPath)}`,
                      { headers }
                    );

                    if (dirResponse.ok) {
                      const dirData = await dirResponse.json();
                      processTreeData(dirData);
                    }
                  } catch (err) {
                    console.warn(`Failed to fetch directory ${dirPath}:`, err);
                  }
                });

                await Promise.all(directoryPromises);

                // Update the already grouped files with accurate sizes
                siblingsWithSizes = siblingsWithSizes.map((file: any) => {
                  if (file.isMultiPart && file.parts) {
                    // Update sizes for multi-part files
                    const updatedParts = file.parts.map((part: any) => ({
                      ...part,
                      size: fileSizeMap.get(part.path) || part.size || 0
                    }));

                    const totalSize = updatedParts.reduce((sum: number, p: any) => sum + p.size, 0);

                    return {
                      ...file,
                      parts: updatedParts,
                      size: totalSize
                    };
                  } else {
                    // Update size for single files
                    const updatedSize = fileSizeMap.get(file.rfilename) || file.size || 0;
                    return {
                      ...file,
                      size: updatedSize
                    };
                  }
                });
              }
            } catch (treeError) {
              console.warn('Failed to fetch file sizes from tree API:', treeError);
            }
          }
          
          // Update the selected model with all detailed information
          setSelectedModel(prev => {
            if (!prev) return prev;
            return {
              ...prev,
              siblings: siblingsWithSizes,
              gguf: detailedModel.gguf || prev.gguf,
              cardData: detailedModel.cardData || prev.cardData,
              lastModified: detailedModel.lastModified || prev.lastModified,
            };
          });
        }
      } catch (error) {
        console.error('Failed to fetch model details:', error);
      } finally {
        setLoadingModelDetails(false);
      }
    };

    fetchModelDetails();
  }, [selectedModel?.id, hfApiKey]);

  // Save API key
  const saveApiKey = async (key: string) => {
    try {
      await fetch('/api/settings/hf-api-key', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ apiKey: key }),
      });
      setHfApiKey(key);
    } catch (error) {
      console.error('Failed to save API key:', error);
    }
  };

  // Search models
  const searchModels = async () => {
    if (!searchQuery.trim()) return;
    
    setLoading(true);
    try {
      const headers: Record<string, string> = {
        'Content-Type': 'application/json',
      };
      
      if (hfApiKey) {
        headers['Authorization'] = `Bearer ${hfApiKey}`;
      }

      // Automatically add "GGUF" to search if not already present since FrogLLM only supports GGUF
      let enhancedQuery = searchQuery.trim();
      if (!enhancedQuery.toLowerCase().includes('gguf')) {
        enhancedQuery = `${enhancedQuery} GGUF`;
      }

      // Build search URL with parameters to include gated models
      const searchParams = new URLSearchParams({
        search: enhancedQuery,
        limit: '50',
        full: 'true',
        filter: 'gguf', // Focus on GGUF format models
      });

      // If authenticated, include gated models in search
      if (hfApiKey) {
        searchParams.append('cardData', 'true'); // Get model card data which includes gated status
      }

      const response = await fetch(
        `https://huggingface.co/api/models?${searchParams.toString()}`,
        { headers }
      );

      if (response.ok) {
        const data = await response.json();
        setModels(data);
      }
    } catch (error) {
      console.error('Error searching models:', error);
    } finally {
      setLoading(false);
    }
  };

  // Start download with destination selection
  const startDownload = async (model: HFModel, filename: string) => {
    // Validate inputs
    if (!filename || filename === 'undefined') {
      console.error('Invalid filename:', filename);
      alert('Error: Invalid filename. Please try again.');
      return;
    }
    
    // Show destination selection modal
    setPendingDownload({ model, filename });
    setShowDestinationModal(true);
  };

  // Start multi-part download
  const startMultiPartDownload = async (model: HFModel, quantization: string, parts: Array<{ path: string; size: number }>) => {
    // Show destination selection modal for multi-part download
    setPendingDownload({ 
      model, 
      filename: quantization, // Use quantization name as display name
      isMultiPart: true,
      parts 
    });
    setShowDestinationModal(true);
  };

  // Actual download function after destination is selected
  const executeDownload = async (destinationPath?: string) => {
    if (!pendingDownload) return;
    
    const { model, filename, isMultiPart, parts } = pendingDownload;
    
    console.log('Starting download:', { modelId: model.id, filename, isMultiPart, destinationPath });
    
    try {
      let requestBody: any;
      
      // Handle multi-part download
      if (isMultiPart && parts) {
        requestBody = {
          modelId: model.id,
          isMultiPart: true,
          quantization: filename, // filename contains quantization name for multi-part
          files: parts.map(p => p.path),
          hfApiKey,
        };
      } else {
        // Handle single file download
        const downloadUrl = `https://huggingface.co/${model.id}/resolve/main/${filename}`;
        requestBody = {
          url: downloadUrl,
          modelId: model.id,
          filename,
          hfApiKey,
        };
      }
      
      // Add destination path if specified
      if (destinationPath) {
        requestBody.destinationPath = destinationPath;
      }
      
      const response = await fetch('/api/models/download', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(requestBody),
      });

      if (response.ok) {
        const result = await response.json();
        // The UI will be updated via real-time events, so we don't need to manually add to downloads
        console.log('Download started:', result);
        
        // Clear pending download and close both modals
        setPendingDownload(null);
        setShowDestinationModal(false);
        setSelectedModel(null); // Close the HuggingFace model details modal
      } else {
        console.error('Failed to start download:', await response.text());
      }
    } catch (error) {
      console.error('Failed to start download:', error);
    }
  };

  // Cancel download
  const cancelDownload = async (downloadId: string) => {
    try {
      await fetch('/api/models/download/cancel', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ downloadId }),
      });
      // The UI will be updated via real-time events
      console.log('Download cancelled:', downloadId);
    } catch (error) {
      console.error('Failed to cancel download:', error);
    }
  };

  // Pause download
  const pauseDownload = async (downloadId: string) => {
    try {
      await fetch(`/api/models/downloads/${downloadId}/pause`, {
        method: 'POST',
      });
      console.log('Download paused:', downloadId);
    } catch (error) {
      console.error('Failed to pause download:', error);
    }
  };

  // Resume download
  const resumeDownload = async (downloadId: string) => {
    try {
      await fetch(`/api/models/downloads/${downloadId}/resume`, {
        method: 'POST',
      });
      console.log('Download resumed:', downloadId);
    } catch (error) {
      console.error('Failed to resume download:', error);
    }
  };

  // Add downloaded model to config
  const addModelToConfig = async (filePath: string) => {
    try {
      console.log('Adding model to config:', filePath);
      
      const response = await fetch('/api/config/append-model', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          filePath,
          options: {
            enableJinja: true,
            throughputFirst: true,
            minContext: 16384,
            preferredContext: 32768,
          }
        }),
      });

      if (response.ok) {
        const result = await response.json();
        console.log('Model added to config:', result);
        
        // Show success notification (you might want to add a toast/notification system)
        alert(`✅ Model "${result.modelInfo.name}" has been added to your configuration and is ready to use!`);
      } else {
        const error = await response.text();
        console.error('Failed to add model to config:', error);
        alert(`❌ Failed to add model to configuration: ${error}`);
      }
    } catch (error) {
      console.error('Failed to add model to config:', error);
      alert(`❌ Failed to add model to configuration: ${error}`);
    }
  };

  // Format file size
  const formatFileSize = (bytes: number) => {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  };

  return (
    <div className="min-h-screen bg-background">
      <div className="max-w-7xl mx-auto px-6 py-8">
        {/* Header */}
        <div className="flex items-center justify-between mb-8">
          <div className="flex items-center gap-4">
            <div className="w-10 h-10 bg-gradient-to-br from-brand-500 to-brand-600 rounded-xl flex items-center justify-center">
              <DatabaseIcon className="w-5 h-5 text-white" />
            </div>
            <div>
              <h1 className="text-2xl font-bold text-text-primary">Model Discovery</h1>
              <p className="text-gray-500 dark:text-gray-300">Browse and download AI models from HuggingFace</p>
            </div>
          </div>
          
          <Button
            variant="ghost"
            onClick={() => setShowSettings(true)}
            className="flex items-center gap-2"
          >
            <SettingsIcon className="w-4 h-4" />
            Settings
          </Button>
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-4 gap-6">
          {/* Sidebar */}
          <div className="lg:col-span-1 space-y-6">
            {/* System Specs */}
            {systemSpecs && (
              <Card>
                <CardHeader>
                  <CardTitle className="flex items-center gap-2">
                    <CpuIcon className="w-4 h-4" />
                    System Info
                  </CardTitle>
                </CardHeader>
                <CardContent className="space-y-3">
                  <div className="flex justify-between text-sm">
                    <span className="text-gray-700 dark:text-gray-300">RAM:</span>
                    <span className="text-gray-900 dark:text-text-primary font-medium">{formatFileSize(systemSpecs.totalRAM)}</span>
                  </div>

                  {/* Multi-GPU Display */}
                  {systemSpecs.gpus && systemSpecs.gpus.length > 0 ? (
                    <div className="space-y-3">
                      <div className="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-2">
                        GPU Resources ({systemSpecs.gpus.length} GPU{systemSpecs.gpus.length > 1 ? 's' : ''})
                      </div>
                      {systemSpecs.gpus.map((gpu, idx) => (
                        <div key={idx} className="bg-surface-secondary p-2 rounded-lg space-y-2">
                          <div className="text-xs font-medium text-gray-900 dark:text-text-primary">
                            GPU {gpu.index}: {gpu.name}
                          </div>
                          <div className="flex items-center gap-2">
                            <div
                              className="flex-1 h-2 bg-background rounded-full overflow-hidden"
                              role="progressbar"
                              aria-label={`GPU ${gpu.index} memory usage`}
                              aria-valuenow={Number(((gpu.memoryUsed / gpu.memoryTotal) * 100).toFixed(0))}
                              aria-valuemin={0}
                              aria-valuemax={100}
                            >
                              <div
                                className="h-full bg-gradient-to-r from-brand-500 to-brand-600"
                                style={{ width: `${((gpu.memoryUsed / gpu.memoryTotal) * 100).toFixed(0)}%` }}
                              />
                            </div>
                            <span className="text-2xs text-gray-700 dark:text-gray-400">
                              {((gpu.memoryUsed / gpu.memoryTotal) * 100).toFixed(0)}%
                            </span>
                          </div>
                          <div className="text-2xs text-gray-700 dark:text-gray-400">
                            {gpu.memoryUsed.toFixed(1)} / {gpu.memoryTotal.toFixed(1)} GB
                          </div>
                        </div>
                      ))}
                      <div className="pt-2 border-t border-border">
                        <div className="flex justify-between text-sm">
                          <span className="text-gray-700 dark:text-gray-300">Total VRAM:</span>
                          <span className="text-gray-900 dark:text-text-primary font-medium">
                            {formatFileSize(systemSpecs.totalVRAM)}
                          </span>
                        </div>
                      </div>
                    </div>
                  ) : (
                    <>
                      <div className="flex justify-between text-sm">
                        <span className="text-gray-700 dark:text-gray-300">VRAM:</span>
                        <span className="text-gray-900 dark:text-text-primary font-medium">{formatFileSize(systemSpecs.totalVRAM)}</span>
                      </div>
                      <div className="flex justify-between text-sm">
                        <span className="text-gray-700 dark:text-gray-300">GPU:</span>
                        <span className="text-gray-900 dark:text-text-primary text-xs font-medium">{systemSpecs.gpuName}</span>
                      </div>
                    </>
                  )}

                  <div className="flex justify-between text-sm">
                    <span className="text-gray-700 dark:text-gray-300">Storage:</span>
                    <span className="text-gray-900 dark:text-text-primary font-medium">{formatFileSize(systemSpecs.diskSpace)}</span>
                  </div>
                </CardContent>
              </Card>
            )}

            {/* Filters */}
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <FilterIcon className="w-4 h-4" />
                  Filters
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className="space-y-2">
                  {filters.map((filter) => (
                    <button
                      key={filter.key}
                      onClick={() => setSelectedFilter(filter.key)}
                      className={`w-full flex items-center justify-between p-3 rounded-lg text-left transition-colors ${
                        selectedFilter === filter.key
                          ? 'text-brand-700 border border-brand-200 dark:bg-brand-900/20 dark:text-brand-300'
                          : 'text-gray-300 hover:text-white hover:bg-surface-secondary'
                      }`}
                    >
                      <span className="text-sm font-medium">{filter.label}</span>
                      <span className="text-xs bg-surface-secondary px-2 py-1 rounded-full">
                        {filter.count}
                      </span>
                    </button>
                  ))}
                </div>
              </CardContent>
            </Card>

            {/* Active Downloads */}
            {downloads.length > 0 && (
              <Card>
                <CardHeader>
                  <CardTitle className="flex items-center gap-2">
                    <DownloadIcon className="w-4 h-4" />
                    Downloads ({downloads.length})
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="space-y-3">
                    {downloads.map((download) => (
                      <div key={`${download.modelId}-${download.filename}`} className="space-y-2">
                        <div className="flex items-center justify-between">
                          <span className="text-sm font-medium text-text-primary truncate">
                            {download.filename}
                          </span>
                          <div className="flex items-center gap-2">
                            {download.status === 'completed' && download.filename.toLowerCase().endsWith('.gguf') && (
                              <Button
                                variant="primary"
                                size="sm"
                                onClick={() => addModelToConfig(download.filePath || `./downloads/${download.filename}`)}
                                className="text-xs"
                              >
                                📋 Add to Config
                              </Button>
                            )}
                            {download.status === 'downloading' && (
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => pauseDownload(download.id)}
                              >
                                ⏸️
                              </Button>
                            )}
                            {download.status === 'paused' && (
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => resumeDownload(download.id)}
                              >
                                ▶️
                              </Button>
                            )}
                            {download.status !== 'completed' && (
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => cancelDownload(download.id)}
                              >
                                <XIcon className="w-3 h-3" />
                              </Button>
                            )}
                          </div>
                        </div>
                        <div
                          className="w-full bg-surface-secondary rounded-full h-2"
                          role="progressbar"
                          aria-label={`Download progress for ${download.filename}`}
                          aria-valuenow={download.progress < 0 || isNaN(download.progress) ? 0 : Math.min(100, Math.max(0, download.progress))}
                          aria-valuemin={0}
                          aria-valuemax={100}
                        >
                          <div
                            className={`h-2 rounded-full transition-all ${
                              download.progress < 0 || isNaN(download.progress)
                                ? 'bg-brand-500 animate-pulse'
                                : 'bg-brand-500'
                            }`}
                            style={{
                              width: download.progress < 0 || isNaN(download.progress)
                                ? '100%'
                                : `${Math.min(100, Math.max(0, download.progress))}%`
                            }}
                          />
                        </div>
                        <div className="flex justify-between text-xs text-gray-700 dark:text-gray-300">
                          <span>
                            {download.progress < 0 || isNaN(download.progress) 
                              ? 'Downloading...' 
                              : `${download.progress.toFixed(1)}%`} • {download.status}
                            {download.retryCount > 0 && (
                              <> • Retry {download.retryCount}</>
                            )}
                            {(() => {
                              // Calculate downloaded bytes from percentage if bytesDownloaded is NaN or invalid
                              let downloadedBytes = download.bytesDownloaded;
                              if (download.totalBytes > 0 && (isNaN(downloadedBytes) || downloadedBytes === undefined)) {
                                // Calculate from percentage
                                downloadedBytes = Math.round((download.progress / 100) * download.totalBytes);
                              }
                              
                              if (download.totalBytes > 0 && !isNaN(downloadedBytes) && downloadedBytes >= 0) {
                                return <> • {formatFileSize(downloadedBytes)} / {formatFileSize(download.totalBytes)}</>;
                              }
                              
                              if (download.totalBytes <= 0 && !isNaN(downloadedBytes) && downloadedBytes > 0) {
                                return <> • {formatFileSize(downloadedBytes)}</>;
                              }
                              
                              return null;
                            })()}
                          </span>
                          <span>
                            {download.speed > 0 && !isNaN(download.speed) && `${formatFileSize(download.speed)}/s`}
                            {download.estimatedTimeRemaining > 0 && download.speed > 0 && !isNaN(download.estimatedTimeRemaining) && (
                              <> • ETA: {Math.ceil(download.estimatedTimeRemaining / 60)}m</>
                            )}
                          </span>
                        </div>
                      </div>
                    ))}
                  </div>
                </CardContent>
              </Card>
            )}
          </div>

          {/* Main Content */}
          <div className="lg:col-span-3 space-y-6">
            {/* Search Bar */}
            <Card>
              <CardContent className="p-6">
                <div className="flex gap-4">
                  <div className="flex-1">
                    <Input
                      placeholder="Search GGUF models... (e.g., 'llama gguf', 'mistral gguf', 'qwen gguf')"
                      value={searchQuery}
                      onChange={(e) => setSearchQuery(e.target.value)}
                      onKeyPress={(e) => e.key === 'Enter' && searchModels()}
                      icon={<SearchIcon className="w-4 h-4" />}
                      className="w-full"
                    />
                  </div>
                  <Button
                    onClick={searchModels}
                    disabled={!searchQuery.trim() || loading}
                    loading={loading}
                    className="flex items-center gap-2"
                  >
                    <SearchIcon className="w-4 h-4" />
                    Search
                  </Button>
                </div>
                
                {!hfApiKey && (
                  <div className="mt-4 p-4 bg-warning-50 border border-warning-200 rounded-lg dark:bg-warning-900/20 dark:border-warning-800">
                    <div className="flex items-start gap-3">
                      <AlertCircleIcon className="w-5 h-5 text-warning-600 dark:text-warning-400 flex-shrink-0 mt-0.5" />
                      <div>
                        <p className="text-sm font-medium text-warning-800 dark:text-warning-200">
                          HuggingFace API Key Required
                        </p>
                        <p className="text-sm text-warning-700 dark:text-warning-300 mt-1">
                          Add your API key in settings to avoid rate limits and access private models.
                        </p>
                        <Button
                          variant="primary"
                          size="sm"
                          onClick={() => setShowSettings(true)}
                          className="mt-3 bg-warning-600 hover:bg-warning-700 border-warning-600 hover:border-warning-700"
                        >
                          <KeyIcon className="w-3 h-3 mr-1" />
                          Configure API Key
                        </Button>
                      </div>
                    </div>
                  </div>
                )}
              </CardContent>
            </Card>

            {/* Models Grid */}
            {filteredModels.length > 0 && (
              <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-6">
                {filteredModels.map((model) => (
                  <motion.div
                    key={model.id}
                    initial={{ opacity: 0, y: 20 }}
                    animate={{ opacity: 1, y: 0 }}
                    whileHover={{ y: -4 }}
                    transition={{ duration: 0.2 }}
                  >
                    <Card variant="elevated" hover className="h-full">
                      <CardHeader>
                        <div className="flex items-start justify-between gap-4">
                          <div className="flex-1 min-w-0">
                            <CardTitle className="text-base truncate">{model.id}</CardTitle>
                            <CardDescription className="text-sm text-gray-600 dark:text-gray-400">
                              by {model.author}
                            </CardDescription>
                          </div>
                          <div className="flex items-center gap-2 text-xs text-gray-600 dark:text-gray-400">
                            <StarIcon className="w-3 h-3" />
                            {model.likes.toLocaleString()}
                          </div>
                        </div>
                      </CardHeader>

                      <CardContent className="space-y-4">
                        {/* Tags */}
                        {model.tags && model.tags.length > 0 && (
                          <div className="flex flex-wrap gap-1">
                            {model.tags.slice(0, 3).map((tag) => (
                              <span
                                key={tag}
                                className="text-xs bg-gray-100 dark:bg-surface-secondary text-gray-700 dark:text-gray-300 px-2 py-1 rounded-full"
                              >
                                {tag}
                              </span>
                            ))}
                            {model.tags.length > 3 && (
                              <span className="text-xs text-gray-600 dark:text-gray-400">
                                +{model.tags.length - 3} more
                              </span>
                            )}
                          </div>
                        )}

                        {/* Stats */}
                        <div className="flex items-center justify-between text-xs text-gray-600 dark:text-gray-300">
                          <div className="flex items-center gap-1">
                            <DownloadIcon className="w-3 h-3" />
                            {model.downloads.toLocaleString()} downloads
                          </div>
                          <div className="flex items-center gap-1">
                            <ClockIcon className="w-3 h-3" />
                            {model.lastModified 
                              ? new Date(model.lastModified).toLocaleDateString()
                              : model.updatedAt 
                                ? new Date(model.updatedAt).toLocaleDateString()
                                : 'N/A'}
                          </div>
                        </div>

                        {/* Model size info */}
                        {(() => {
                          const ggufFiles = model.siblings?.filter(s => 
                            s.rfilename.toLowerCase().endsWith('.gguf')
                          ) || [];
                          
                          return ggufFiles.length > 0 && (
                            <div className="text-xs text-gray-600 dark:text-gray-300">
                              <div className="flex items-center gap-1 mb-1">
                                <HardDriveIcon className="w-3 h-3" />
                                {ggufFiles.length} GGUF {ggufFiles.length === 1 ? 'file' : 'files'}
                              </div>
                              {ggufFiles.some(s => s.size) && (
                                <div className="text-gray-600 dark:text-gray-400">
                                  Total: {formatFileSize(
                                    ggufFiles.reduce((sum, s) => sum + (s.size || 0), 0)
                                  )}
                                </div>
                              )}
                            </div>
                          );
                        })()}

                        {/* Actions */}
                        <div className="flex gap-2 pt-2">
                          <Button
                            variant="primary"
                            size="sm"
                            onClick={() => setSelectedModel(model)}
                            className="flex-1"
                          >
                            <DownloadIcon className="w-3 h-3 mr-1" />
                            Download
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => window.open(`https://huggingface.co/${model.id}`, '_blank')}
                          >
                            <ExternalLinkIcon className="w-3 h-3" />
                          </Button>
                        </div>
                      </CardContent>
                    </Card>
                  </motion.div>
                ))}
              </div>
            )}

            {/* No results */}
            {!loading && models.length === 0 && searchQuery && (
              <Card>
                <CardContent className="text-center py-12">
                  <DatabaseIcon className="w-16 h-16 text-gray-500 dark:text-gray-400 mx-auto mb-4" />
                  <h3 className="text-lg font-semibold text-text-primary mb-2">No models found</h3>
                  <p className="text-gray-600 dark:text-gray-300">
                    Try searching for different keywords or check your spelling.
                  </p>
                </CardContent>
              </Card>
            )}

            {/* Getting started */}
            {!searchQuery && (
              <Card>
                <CardContent className="text-center py-12">
                  <SearchIcon className="w-16 h-16 text-gray-500 dark:text-gray-400 mx-auto mb-4" />
                  <h3 className="text-lg font-semibold text-text-primary mb-2">Discover AI Models</h3>
                  <p className="text-gray-600 dark:text-gray-300 mb-4">
                    Search for AI models from HuggingFace Hub. Try popular models like "llama", "mistral", or "code".
                  </p>
                  <div className="flex flex-wrap justify-center gap-2">
                    {['llama gguf', 'mistral gguf', 'qwen gguf', 'phi gguf', 'gemma gguf'].map((suggestion) => (
                      <Button
                        key={suggestion}
                        variant="outline"
                        size="sm"
                        onClick={() => {
                          setSearchQuery(suggestion);
                          // Use setTimeout to ensure state is updated before search
                          setTimeout(() => searchModels(), 0);
                        }}
                      >
                        {suggestion}
                      </Button>
                    ))}
                  </div>
                </CardContent>
              </Card>
            )}
          </div>
        </div>
      </div>

      {/* Settings Modal */}
      <Modal
        open={showSettings}
        onClose={() => setShowSettings(false)}
        title="Download Settings"
        description="Configure your HuggingFace API key and download preferences"
        size="md"
      >
        <div className="space-y-6">
          <div>
            <label className="block text-sm font-medium text-text-primary mb-2">
              HuggingFace API Key
            </label>
            <Input
              type="password"
              placeholder="hf_xxxxxxxxxxxxxxxxxxxx"
              value={hfApiKey}
              onChange={(e) => setHfApiKey(e.target.value)}
              icon={<KeyIcon className="w-4 h-4" />}
              helper="Get your API key from https://huggingface.co/settings/tokens"
            />
          </div>

          <div className="flex justify-end gap-3">
            <Button variant="ghost" onClick={() => setShowSettings(false)}>
              Cancel
            </Button>
            <Button 
              variant="primary" 
              onClick={() => {
                saveApiKey(hfApiKey);
                setShowSettings(false);
              }}
            >
              Save Settings
            </Button>
          </div>
        </div>
      </Modal>

      {/* Model Details Modal */}
      {selectedModel && (
        <Modal
          open={!!selectedModel}
          onClose={() => setSelectedModel(null)}
          title={selectedModel.id}
          description={`by ${selectedModel.author}`}
          size="lg"
        >
          <div className="space-y-6">
            {/* Model Info */}
            <div className="grid grid-cols-2 gap-4 text-sm">
              <div>
                <span className="text-gray-600 dark:text-gray-300">Downloads:</span>
                <span className="ml-2 text-text-primary">{selectedModel.downloads.toLocaleString()}</span>
              </div>
              <div>
                <span className="text-gray-600 dark:text-gray-300">Likes:</span>
                <span className="ml-2 text-text-primary">{selectedModel.likes.toLocaleString()}</span>
              </div>
              <div>
                <span className="text-gray-600 dark:text-gray-300">Updated:</span>
                <span className="ml-2 text-text-primary">
                  {(() => {
                    const dateStr = selectedModel.lastModified || selectedModel.updatedAt;
                    if (dateStr && dateStr !== 'Invalid Date') {
                      try {
                        return new Date(dateStr).toLocaleDateString(undefined, { 
                          year: 'numeric', 
                          month: 'short', 
                          day: 'numeric' 
                        });
                      } catch {
                        return 'N/A';
                      }
                    }
                    return 'N/A';
                  })()}
                </span>
              </div>
              <div>
                <span className="text-gray-600 dark:text-gray-300">License:</span>
                <span className="ml-2 text-text-primary">{selectedModel.cardData?.license || 'N/A'}</span>
              </div>
            </div>

            {/* GGUF Model Info */}
            {selectedModel.gguf && (
              <div className="p-4 bg-brand-50 dark:bg-brand-900/20 border border-brand-200 dark:border-brand-800 rounded-lg">
                <h4 className="text-sm font-semibold text-text-primary mb-3 flex items-center gap-2">
                  <DatabaseIcon className="w-4 h-4" />
                  GGUF Model Information
                </h4>
                <div className="grid grid-cols-2 gap-3 text-xs">
                  {selectedModel.gguf.architecture && (
                    <div>
                      <span className="text-gray-600 dark:text-gray-300">Architecture:</span>
                      <span className="ml-2 text-blue-400 font-medium">{selectedModel.gguf.architecture}</span>
                    </div>
                  )}
                  {selectedModel.gguf.context_length && (
                    <div>
                      <span className="text-gray-600 dark:text-gray-300">Context Length:</span>
                      <span className="ml-2 text-blue-400 font-medium">{selectedModel.gguf.context_length.toLocaleString()} tokens</span>
                    </div>
                  )}
                  {selectedModel.gguf.total && (
                    <div className="col-span-2">
                      <span className="text-gray-600 dark:text-gray-300">Total Model Size:</span>
                      <span className="ml-2 text-blue-400 font-medium">{formatFileSize(selectedModel.gguf.total)}</span>
                    </div>
                  )}
                  {selectedModel.cardData?.base_model && selectedModel.cardData.base_model.length > 0 && (
                    <div className="col-span-2">
                      <span className="text-gray-600 dark:text-gray-300">Base Model:</span>
                      <span className="ml-2 text-blue-400 font-medium">{selectedModel.cardData.base_model[0]}</span>
                    </div>
                  )}
                </div>
              </div>
            )}

            {/* System Resources Info */}
            {systemSpecs && (
              <div className="p-4 bg-brand-50 dark:bg-brand-900/20 border border-brand-200 dark:border-brand-800 rounded-lg">
                <h4 className="text-sm font-semibold text-text-primary mb-3 flex items-center gap-2">
                  <CpuIcon className="w-4 h-4" />
                  Your System Resources
                </h4>
                <div className="grid grid-cols-2 gap-3 text-xs">
                  <div>
                    <span className="text-gray-600 dark:text-gray-300">RAM:</span>
                    <span className="ml-2 text-blue-400 font-medium">{formatFileSize(systemSpecs.totalRAM)}</span>
                  </div>
                  <div>
                    <span className="text-gray-600 dark:text-gray-300">VRAM:</span>
                    <span className="ml-2 text-blue-400 font-medium">{formatFileSize(systemSpecs.totalVRAM)}</span>
                  </div>
                  <div className="col-span-2">
                    <span className="text-gray-600 dark:text-gray-300">GPU:</span>
                    <span className="ml-2 text-blue-400 font-medium">{systemSpecs.gpuName || 'N/A'}</span>
                  </div>
                </div>
              </div>
            )}

            {/* Files */}
            <div>
              <h4 className="text-sm font-semibold text-text-primary mb-3 flex items-center gap-2">
                Available GGUF Files
                {loadingModelDetails && (
                  <div className="animate-spin rounded-full h-3 w-3 border-b-2 border-brand-500"></div>
                )}
              </h4>
              {(() => {
                // Get all GGUF files (both single and multi-part)
                const ggufFiles = selectedModel.siblings || [];
                
                return ggufFiles.length > 0 ? (
                  <div className="space-y-2 max-h-60 overflow-y-auto">
                    {ggufFiles.map((file: any) => (
                    <div
                      key={file.rfilename}
                      className="flex items-center justify-between p-3 bg-surface-secondary rounded-lg hover:bg-surface-tertiary transition-colors"
                    >
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2">
                          <p className="text-sm font-medium text-text-primary truncate">
                            {file.rfilename}
                          </p>
                          {file.isMultiPart && (
                            <span className="px-2 py-0.5 text-xs bg-brand-100 dark:bg-brand-900/30 text-brand-700 dark:text-brand-300 rounded-full whitespace-nowrap">
                              {file.partCount} parts
                            </span>
                          )}
                        </div>
                        <p className="text-xs text-gray-600 dark:text-gray-300 mt-1">
                          {loadingModelDetails ? (
                            <span className="flex items-center gap-1">
                              <span className="animate-pulse">Loading size...</span>
                            </span>
                          ) : (
                            <>
                              {file.size ? formatFileSize(file.size) : 'Size unknown'}
                              {file.isMultiPart && file.partCount && (
                                <span className="ml-2 text-gray-600 dark:text-gray-400">
                                  ({file.partCount} {file.partCount === 1 ? 'file' : 'files'})
                                </span>
                              )}
                            </>
                          )}
                        </p>
                      </div>
                      <Button
                        variant="primary"
                        size="sm"
                        onClick={() => {
                          // For multi-part files, pass the parts array
                          if (file.isMultiPart && file.parts) {
                            startMultiPartDownload(selectedModel, file.rfilename, file.parts);
                          } else {
                            startDownload(selectedModel, file.rfilename);
                          }
                        }}
                        className="ml-3"
                      >
                        <DownloadIcon className="w-3 h-3 mr-1" />
                        Download
                      </Button>
                    </div>
                  ))}
                  </div>
                ) : (
                  <div className="text-center py-8 text-gray-600 dark:text-gray-300">
                    <p>No GGUF files found for this model.</p>
                    <p className="text-xs mt-1">FrogLLM only supports GGUF format models. Try searching for models with "GGUF" in the name.</p>
                  </div>
                );
              })()}
            </div>

            <div className="flex justify-end gap-3">
              <Button variant="ghost" onClick={() => setSelectedModel(null)}>
                Close
              </Button>
              <Button
                variant="outline"
                onClick={() => window.open(`https://huggingface.co/${selectedModel.id}`, '_blank')}
              >
                <ExternalLinkIcon className="w-4 h-4 mr-2" />
                View on HuggingFace
              </Button>
            </div>
          </div>
        </Modal>
      )}

      {/* Download Destination Modal */}
      <DownloadDestinationModal
        open={showDestinationModal}
        onClose={() => {
          setShowDestinationModal(false);
          setPendingDownload(null);
        }}
        onSelect={executeDownload}
        modelName={pendingDownload?.model.id || ''}
        filename={pendingDownload?.filename || ''}
      />
    </div>
  );
};

export default ModelDownloaderPage;