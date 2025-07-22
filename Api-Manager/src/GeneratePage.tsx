import React, { useState, useEffect } from 'react';
import { motion } from 'framer-motion';
import { Button, Card, Input, Select, Alert, TagInput } from './ui';
import { generateApiKey, listTemplates, getDefaultTemplate } from './api';
import { APIKey, KeyTemplate, Tag } from './types';

const GeneratePage: React.FC = () => {
  // Initialize with safe default values
  const [formState, setFormState] = useState({
    name: '',
    duration: '30d',
    rpm: '60', // Default value
    threadsLimit: '5', // Default value
    totalRequests: '0',
    apiKey: '',
  });
  
  const [selectedTemplate, setSelectedTemplate] = useState<string>('');
  const [templates, setTemplates] = useState<KeyTemplate[]>([]);
  const [tags, setTags] = useState<Tag[]>([]);
  const [generatedKey, setGeneratedKey] = useState<APIKey | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showCopySuccess, setShowCopySuccess] = useState(false);

  // Load templates on mount
  useEffect(() => {
    const fetchTemplates = async () => {
      try {
        const templateData = await listTemplates();
        setTemplates(templateData || []);
        
        // Safely get default template
        try {
          const defaultTemplate = await getDefaultTemplate();
          if (defaultTemplate) {
            applyTemplate(defaultTemplate);
            setSelectedTemplate(defaultTemplate.id || '');
          }
        } catch (err) {
          console.error('Failed to fetch default template:', err);
        }
      } catch (err) {
        console.error('Failed to fetch templates:', err);
        setError('Failed to load templates. Please try again later.');
      }
    };
    
    fetchTemplates();
  }, []);

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
    const { name, value } = e.target;
    setFormState((prev) => ({ ...prev, [name]: value }));
  };

  const applyTemplate = (template: KeyTemplate | null) => {
    if (!template) return;
    
    setFormState((prev) => ({
      ...prev,
      rpm: template.rpm !== undefined ? template.rpm.toString() : prev.rpm,
      threadsLimit: template.threads_limit !== undefined ? template.threads_limit.toString() : prev.threadsLimit,
      duration: template.duration || prev.duration,
    }));
    
    if (template.tags) {
      setTags(template.tags);
    }
  };

  const handleTemplateChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    const templateId = e.target.value;
    setSelectedTemplate(templateId);
    
    if (!templateId) return;
    
    const template = templates.find((t) => t.id === templateId);
    if (template) {
      applyTemplate(template);
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setIsLoading(true);
    
    try {
      const params: Record<string, string> = {
        name: formState.name,
        expiration: formState.duration,
        rpm: formState.rpm,
        tl: formState.threadsLimit,
        limit: formState.totalRequests,
      };
      
      // Add custom API key if provided
      if (formState.apiKey) {
        params.apikey = formState.apiKey;
      }
      
      // Add tags if present
      if (tags.length > 0) {
        tags.forEach((tag, index) => {
          params[`tags[${index}]`] = tag.name;
        });
      }
      
      const result = await generateApiKey(params);
      setGeneratedKey(result);
      
      // Reset form if successful
      setFormState({
        name: '',
        duration: '30d',
        rpm: '60',
        threadsLimit: '5',
        totalRequests: '0',
        apiKey: '',
      });
      setTags([]);
      setSelectedTemplate('');
    } catch (err) {
      console.error('Failed to generate API key:', err);
      setError('Failed to generate API key. Please check your input and try again.');
    } finally {
      setIsLoading(false);
    }
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text).then(
      () => {
        setShowCopySuccess(true);
        setTimeout(() => setShowCopySuccess(false), 2000);
      },
      (err) => {
        console.error('Could not copy text: ', err);
      }
    );
  };

  const formatDate = (dateString: string) => {
    if (!dateString) return 'N/A';
    try {
      const date = new Date(dateString);
      return date.toLocaleString();
    } catch (e) {
      return dateString;
    }
  };

  return (
    <div className="container mx-auto px-4">
      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.3 }}
      >
        <h1 className="text-2xl md:text-3xl font-bold text-gray-900 dark:text-white mb-6">Generate API Key</h1>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-8">
          {/* Form Card */}
          <Card title="Key Configuration" className="h-auto">
            {error && (
              <Alert
                type="error"
                message={error}
                onClose={() => setError(null)}
                className="mb-4"
              />
            )}

            <form onSubmit={handleSubmit}>
              <Input
                label="Key Name"
                name="name"
                value={formState.name}
                onChange={handleInputChange}
                placeholder="Enter a descriptive name"
                className="mb-4"
              />

              <Select
                label="Template"
                name="template"
                value={selectedTemplate}
                onChange={handleTemplateChange}
                options={[
                  { value: '', label: 'Select a template' },
                  ...templates.map((template) => ({
                    value: template.id || '',
                    label: template.name,
                  })),
                ]}
                className="mb-4"
              />

              <Select
                label="Expiration"
                name="duration"
                value={formState.duration}
                onChange={handleInputChange}
                options={[
                  { value: '1h', label: '1 Hour' },
                  { value: '24h', label: '24 Hours' },
                  { value: '7d', label: '7 Days' },
                  { value: '30d', label: '30 Days' },
                  { value: '90d', label: '90 Days' },
                  { value: '365d', label: '1 Year' },
                ]}
                className="mb-4"
              />

              <Input
                label="Rate Limit (requests per minute)"
                name="rpm"
                type="number"
                value={formState.rpm}
                onChange={handleInputChange}
                placeholder="e.g. 60"
                className="mb-4"
              />

              <Input
                label="Concurrency Limit (max threads)"
                name="threadsLimit"
                type="number"
                value={formState.threadsLimit}
                onChange={handleInputChange}
                placeholder="e.g. 5"
                className="mb-4"
              />

              <Input
                label="Total Requests Limit (0 for unlimited)"
                name="totalRequests"
                type="number"
                value={formState.totalRequests}
                onChange={handleInputChange}
                placeholder="e.g. 1000"
                className="mb-4"
              />

              <Input
                label="Custom API Key (optional)"
                name="apiKey"
                value={formState.apiKey}
                onChange={handleInputChange}
                placeholder="Leave blank to generate random key"
                className="mb-4"
              />

              <TagInput
                label="Tags"
                value={tags}
                onChange={setTags}
                placeholder="Type tag name and press Enter..."
                className="mb-6"
              />

              <Button
                type="submit"
                variant="primary"
                className="w-full"
                disabled={isLoading}
              >
                {isLoading ? (
                  <div className="flex items-center justify-center">
                    <svg
                      className="animate-spin -ml-1 mr-2 h-4 w-4 text-white"
                      xmlns="http://www.w3.org/2000/svg"
                      fill="none"
                      viewBox="0 0 24 24"
                    >
                      <circle
                        className="opacity-25"
                        cx="12"
                        cy="12"
                        r="10"
                        stroke="currentColor"
                        strokeWidth="4"
                      ></circle>
                      <path
                        className="opacity-75"
                        fill="currentColor"
                        d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                      ></path>
                    </svg>
                    Generating...
                  </div>
                ) : (
                  'Generate API Key'
                )}
              </Button>
            </form>
          </Card>

          {/* Result Card */}
          <Card
            title="Generated API Key"
            className={`h-auto ${!generatedKey ? 'flex items-center justify-center' : ''}`}
          >
            {generatedKey ? (
              <motion.div
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                transition={{ duration: 0.3 }}
                className="space-y-4"
              >
                <div className="p-4 bg-gray-50 dark:bg-gray-700 rounded-md relative">
                  <div className="font-mono text-sm break-all">{generatedKey.key}</div>
                  <button
                    onClick={() => copyToClipboard(generatedKey.key)}
                    className="absolute top-2 right-2 p-2 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
                    title="Copy to clipboard"
                  >
                    <svg
                      className="w-5 h-5"
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                      xmlns="http://www.w3.org/2000/svg"
                    >
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                        d="M8 5H6a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2v-1M8 5a2 2 0 002 2h2a2 2 0 002-2M8 5a2 2 0 012-2h2a2 2 0 012 2m0 0h2a2 2 0 012 2v3m2 4H10m0 0l3-3m-3 3l3 3"
                      />
                    </svg>
                  </button>
                  {showCopySuccess && (
                    <div className="absolute -top-8 right-0 bg-green-500 text-white px-2 py-1 rounded text-xs">
                      Copied!
                    </div>
                  )}
                </div>

                <div className="space-y-2">
                  {generatedKey.name && (
                    <div className="flex justify-between">
                      <span className="text-gray-600 dark:text-gray-400">Name:</span>
                      <span className="font-medium">{generatedKey.name}</span>
                    </div>
                  )}
                  <div className="flex justify-between">
                    <span className="text-gray-600 dark:text-gray-400">Expires:</span>
                    <span className="font-medium">{formatDate(generatedKey.expiration)}</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-gray-600 dark:text-gray-400">Rate Limit:</span>
                    <span className="font-medium">{generatedKey.rpm || 0} req/min</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-gray-600 dark:text-gray-400">Thread Limit:</span>
                    <span className="font-medium">{generatedKey.threads_limit || 0}</span>
                  </div>
                  {(generatedKey.total_requests || 0) > 0 && (
                    <div className="flex justify-between">
                      <span className="text-gray-600 dark:text-gray-400">Request Limit:</span>
                      <span className="font-medium">{generatedKey.total_requests}</span>
                    </div>
                  )}
                  {generatedKey.tags && generatedKey.tags.length > 0 && (
                    <div className="flex justify-between">
                      <span className="text-gray-600 dark:text-gray-400">Tags:</span>
                      <div className="flex flex-wrap gap-1 justify-end">
                        {generatedKey.tags.map((tag) => (
                          <span
                            key={tag.name}
                            className="px-2 py-1 text-xs rounded-full"
                            style={{
                              backgroundColor: tag.color || '#e5e7eb',
                              color: '#1f2937',
                            }}
                          >
                            {tag.name}
                          </span>
                        ))}
                      </div>
                    </div>
                  )}
                </div>

                <div className="pt-4 border-t border-gray-200 dark:border-gray-700">
                  <Button
                    variant="outline"
                    className="w-full"
                    onClick={() => {
                      copyToClipboard(generatedKey.key);
                    }}
                  >
                    Copy API Key
                  </Button>
                  <Button
                    variant="outline"
                    className="w-full mt-2"
                    onClick={() => setGeneratedKey(null)}
                  >
                    Clear
                  </Button>
                </div>
              </motion.div>
            ) : (
              <div className="text-center text-gray-500 dark:text-gray-400 py-8">
                <svg
                  className="mx-auto h-12 w-12 text-gray-400 dark:text-gray-600"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                  xmlns="http://www.w3.org/2000/svg"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z"
                  />
                </svg>
                <p className="mt-2">Generated API key will appear here</p>
              </div>
            )}
          </Card>
        </div>
      </motion.div>
    </div>
  );
};

export default GeneratePage;