export default {
  batchImageGuide: {
    title: 'Batch Image Generation',
    description: 'Submit multiple prompts in one job and download the generated images when complete'
  },
  // Home Page
  home: {
    viewDocs: 'View Documentation',
    docs: 'Docs',
    switchToLight: 'Switch to Light Mode',
    switchToDark: 'Switch to Dark Mode',
    dashboard: 'Dashboard',
    login: 'Login',
    register: 'Create account',
    startNow: 'Get started',
    loginConsole: 'Sign in to Console',
    goToDashboard: 'Go to Dashboard',
    customPageTitle: 'Custom Home Content',
    defaultSubtitle: 'Stable, transparent, and manageable intelligent services',
    nav: {
      primary: 'Public navigation',
      toggle: 'Open or close the navigation menu',
      overview: 'Overview',
      reliability: 'Reliable service',
      guide: 'Guide',
      faq: 'FAQ',
      docs: 'Documentation'
    },
    hero: {
      eyebrow: 'Stable access · Clear usage · Flexible control',
      titlePrimary: 'Make services more reliable',
      titleSecondary: 'Make every use more valuable',
      description:
        'Use one credential for conversation, reasoning, code, image, and video capabilities. Review status, usage, and cost in one place for individual or team workflows.',
      visualAlt: 'Digital service capabilities and operational data visualization',
      secondaryAction: 'Explore capabilities',
      assurances: {
        clearUsage: 'Clear usage records',
        flexibleAccess: 'Flexible access scopes',
        visibleStatus: 'Visible service status'
      }
    },
    flow: {
      access: 'Unified access',
      routing: 'Flexible routing',
      service: 'Capabilities',
      records: 'Usage records'
    },
    overview: {
      eyebrow: 'Overview',
      title: 'Manage first-time access and everyday operations in one place',
      description:
        'Credentials, service paths, usage records, and team permissions stay together, reducing repeated setup and keeping every request accountable.',
      items: {
        access: {
          title: 'One credential system',
          description: 'Create credentials per project and assign an independent access scope to each use case.'
        },
        routing: {
          title: 'Flexible service paths',
          description: 'Match requests with an available path and reduce the impact of an isolated service issue.'
        },
        usage: {
          title: 'Transparent usage and cost',
          description: 'Review request status, usage details, and cost records without switching between systems.'
        },
        control: {
          title: 'Scoped permissions and quotas',
          description: 'Set access boundaries, quotas, and expiration dates for individual and team use.'
        }
      },
      catalogLabel: 'Available capabilities follow the live Console catalog',
      catalog: {
        conversation: 'Conversation and reasoning',
        code: 'Code collaboration',
        image: 'Image creation',
        video: 'Video creation',
        tools: 'Search and tools'
      }
    },
    reliability: {
      eyebrow: 'Reliable service',
      title: 'Visible status and complete records for everyday operations',
      description:
        'The platform brings service state and request outcomes together and routes across available paths so investigation and follow-up have clear context.',
      action: 'View the guide',
      items: {
        status: {
          title: 'Service status in one place',
          description: 'Review available capabilities and channel state before choosing how to run a request.'
        },
        continuity: {
          title: 'Automatic path adjustment',
          description: 'When the current path is unavailable, configured policy can try another available path.'
        },
        records: {
          title: 'Request records remain available',
          description: 'Keep status, usage, and error context together when reviewing results or client setup.'
        }
      }
    },
    guide: {
      eyebrow: 'Guide',
      title: 'Complete the initial setup in three steps',
      description: 'Create an account, issue a credential, and configure the service address required by your workflow.',
      docsAction: 'Read the complete guide',
      items: {
        account: {
          title: 'Create and sign in to an account',
          description: 'Enter the Console to review currently available features and service status.'
        },
        credential: {
          title: 'Issue an access credential',
          description: 'Create a credential for the intended use and set its scope, quota, and expiration.'
        },
        configure: {
          title: 'Follow the configuration guide',
          description: 'Set the service address and credential while keeping the rest of your workflow unchanged.'
        }
      }
    },
    faq: {
      eyebrow: 'FAQ',
      title: 'What to know before getting started',
      description: 'Use the signed-in Console as the source of truth for current capabilities, pricing, and status.',
      items: {
        start: {
          question: 'How do I get started?',
          answer: 'Create an account, open the Console, issue an access credential, and follow the guide to configure the service address.'
        },
        capabilities: {
          question: 'Which capabilities are available?',
          answer: 'The catalog can include conversation, reasoning, code collaboration, image creation, video creation, search, and tools. Check the Console for the current list.'
        },
        usage: {
          question: 'Where can I review usage and cost?',
          answer: 'The signed-in Console provides balance, request records, usage details, and cost information.'
        },
        failure: {
          question: 'How should I investigate a failed request?',
          answer: 'Check service status and request records first, then verify the credential scope, quota, and client configuration against the reported error.'
        }
      }
    },
    cta: {
      eyebrow: 'Get started',
      title: 'Ready for a unified intelligent service experience?',
      description: 'Create an account, open the Console, and follow the guide to complete the initial setup.',
      docsAction: 'Read the guide'
    },
    footer: {
      tagline: 'Stable, transparent, and manageable intelligent services',
      allRightsReserved: 'All rights reserved.'
    }
  },

  // Key Usage Query Page
  keyUsage: {
    title: 'API Key Usage',
    subtitle: 'Enter your API Key to view real-time spending and usage status',
    placeholder: 'sk-ant-mirror-xxxxxxxxxxxx',
    query: 'Query',
    querying: 'Querying...',
    privacyNote: 'Your Key is processed locally in the browser and will not be stored',
    dateRange: 'Date Range:',
    dateRangeToday: 'Today',
    dateRange7d: '7 Days',
    dateRange30d: '30 Days',
    dateRange90d: '90 Days',
    dateRangeCustom: 'Custom',
    apply: 'Apply',
    used: 'Used',
    detailInfo: 'Detail Information',
    tokenStats: 'Token Statistics',
    dailyDetail: 'Daily Detail',
    modelStats: 'Model Usage Statistics',
    // Table headers
    date: 'Date',
    model: 'Model',
    requests: 'Requests',
    inputTokens: 'Input Tokens',
    outputTokens: 'Output Tokens',
    cacheCreationTokens: 'Cache Creation',
    cacheReadTokens: 'Cache Read',
    cacheWriteTokens: 'Cache Write',
    totalTokens: 'Total Tokens',
    cost: 'Cost',
    // Status
    quotaMode: 'Key Quota Mode',
    walletBalance: 'Wallet Balance',
    // Ring card titles
    totalQuota: 'Total Quota',
    limit5h: '5-Hour Limit',
    limitDaily: 'Daily Limit',
    limit7d: '7-Day Limit',
    limitWeekly: 'Weekly Limit',
    limitMonthly: 'Monthly Limit',
    // Detail rows
    remainingQuota: 'Remaining Quota',
    expiresAt: 'Expires At',
    todayExpires: '(expires today)',
    daysLeft: '({days} days)',
    usedQuota: 'Used Quota',
    resetNow: 'Resetting soon',
    subscriptionType: 'Subscription Type',
    subscriptionExpires: 'Subscription Expires',
    // Usage stat cells
    todayRequests: 'Today Requests',
    todayInputTokens: 'Today Input',
    todayOutputTokens: 'Today Output',
    todayTokens: 'Today Tokens',
    todayCacheCreation: 'Today Cache Creation',
    todayCacheRead: 'Today Cache Read',
    todayCost: 'Today Cost',
    rpmTpm: 'RPM / TPM',
    totalRequests: 'Total Requests',
    totalInputTokens: 'Total Input',
    totalOutputTokens: 'Total Output',
    totalTokensLabel: 'Total Tokens',
    totalCacheCreation: 'Total Cache Creation',
    totalCacheRead: 'Total Cache Read',
    totalCost: 'Total Cost',
    avgDuration: 'Avg Duration',
    // Messages
    enterApiKey: 'Please enter an API Key',
    querySuccess: 'Query successful',
    queryFailed: 'Query failed',
    queryFailedRetry: 'Query failed, please try again later',
    noDailyUsage: 'No daily usage data',
  },

  // Setup Wizard
  setup: {
    title: 'Sub2API Setup',
    description: 'Configure your Sub2API instance',
    database: {
      title: 'Database Configuration',
      description: 'Connect to your PostgreSQL database',
      host: 'Host',
      port: 'Port',
      username: 'Username',
      password: 'Password',
      databaseName: 'Database Name',
      sslMode: 'SSL Mode',
      passwordPlaceholder: 'Password',
      ssl: {
        disable: 'Disable',
        require: 'Require',
        verifyCa: 'Verify CA',
        verifyFull: 'Verify Full'
      }
    },
    redis: {
      title: 'Redis Configuration',
      description: 'Connect to your Redis server',
      host: 'Host',
      port: 'Port',
      username: 'Username (optional)',
      password: 'Password (optional)',
      database: 'Database',
      usernamePlaceholder: 'Leave empty for default user',
      passwordPlaceholder: 'Password',
      enableTls: 'Enable TLS',
      enableTlsHint: 'Use TLS when connecting to Redis (public CA certs)'
    },
    admin: {
      title: 'Admin Account',
      description: 'Create your administrator account',
      email: 'Email',
      password: 'Password',
      confirmPassword: 'Confirm Password',
      passwordPlaceholder: 'Min 8 characters',
      confirmPasswordPlaceholder: 'Confirm password',
      passwordMismatch: 'Passwords do not match'
    },
    ready: {
      title: 'Ready to Install',
      description: 'Review your configuration and complete setup',
      database: 'Database',
      redis: 'Redis',
      adminEmail: 'Admin Email'
    },
    status: {
      testing: 'Testing...',
      success: 'Connection Successful',
      testConnection: 'Test Connection',
      installing: 'Installing...',
      completeInstallation: 'Complete Installation',
      completed: 'Installation completed!',
      redirecting: 'Redirecting to login page...',
      restarting: 'Service is restarting, please wait...',
      timeout: 'Service restart is taking longer than expected. Please refresh the page manually.'
    }
  },

  // Common
}
