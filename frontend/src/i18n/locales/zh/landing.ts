export default {
  batchImageGuide: {
    title: '图片批量生成',
    description: '一次提交多条提示词，任务完成后可统一下载图片结果'
  },
  // Home Page
  home: {
    viewDocs: '查看文档',
    docs: '文档',
    switchToLight: '切换到浅色模式',
    switchToDark: '切换到深色模式',
    dashboard: '控制台',
    login: '登录',
    register: '注册',
    startNow: '立即开始',
    loginConsole: '登录控制台',
    goToDashboard: '进入控制台',
    customPageTitle: '自定义首页内容',
    defaultSubtitle: '稳定、清晰、可管理的智能服务平台',
    nav: {
      primary: '公开导航',
      toggle: '打开或关闭导航菜单',
      overview: '功能概览',
      reliability: '稳定服务',
      guide: '使用指南',
      faq: '常见问题',
      docs: '文档'
    },
    hero: {
      eyebrow: '稳定连接 · 清晰计量 · 灵活管理',
      description:
        '用一套访问凭证连接对话、推理、代码、图像与视频能力。状态、用量和费用集中查看，个人与团队都能快速开始。',
      secondaryAction: '查看功能概览',
      assurances: {
        clearUsage: '用量明细清晰可查',
        flexibleAccess: '访问范围灵活配置',
        visibleStatus: '服务状态集中查看'
      }
    },
    flow: {
      access: '统一接入',
      routing: '灵活调度',
      service: '能力服务',
      records: '用量记录'
    },
    overview: {
      eyebrow: '功能概览',
      title: '从首次接入到日常管理，都在同一处完成',
      description:
        '平台将访问凭证、服务路径、用量记录和团队权限集中管理，减少重复配置，让每一次使用都有清晰依据。',
      items: {
        access: {
          title: '一套凭证，统一管理',
          description: '按项目创建和管理访问凭证，并为不同用途配置独立的访问范围。'
        },
        routing: {
          title: '服务路径灵活选择',
          description: '根据当前可用状态匹配合适路径，降低单一路径异常带来的影响。'
        },
        usage: {
          title: '用量与费用清晰可查',
          description: '集中查看请求状态、使用明细和费用记录，日常核对更直接。'
        },
        control: {
          title: '权限与额度按需分配',
          description: '为个人或团队设置访问边界、额度和有效期，控制使用范围。'
        }
      },
      catalogLabel: '可用能力以控制台实时展示为准',
      catalog: {
        conversation: '对话与推理',
        code: '代码协作',
        image: '图像创作',
        video: '视频创作',
        tools: '搜索与工具'
      }
    },
    reliability: {
      eyebrow: '稳定服务',
      title: '用可见状态与完整记录支撑日常使用',
      description:
        '平台持续汇总服务状态与请求结果，并在可用路径之间进行调度，让问题定位和后续处理都有依据。',
      action: '查看使用指南',
      items: {
        status: {
          title: '服务状态集中展示',
          description: '在同一页面查看可用能力和渠道状态，选择服务时更有把握。'
        },
        continuity: {
          title: '异常路径自动调整',
          description: '当当前路径不可用时，平台可按既定策略尝试其他可用路径。'
        },
        records: {
          title: '请求记录持续可查',
          description: '保留状态、用量与错误信息，便于核对结果并排查客户端配置。'
        }
      }
    },
    guide: {
      eyebrow: '使用指南',
      title: '三步完成首次配置',
      description: '从创建账号到发起首次请求，只需完成必要的账号、凭证和服务地址配置。',
      docsAction: '阅读完整使用文档',
      items: {
        account: {
          title: '创建并登录账号',
          description: '完成账号注册后进入控制台，查看当前可用功能和服务状态。'
        },
        credential: {
          title: '生成访问凭证',
          description: '按用途创建凭证，并设置需要的访问范围、额度与有效期。'
        },
        configure: {
          title: '按文档完成配置',
          description: '填写服务地址和访问凭证，保留原有业务流程即可开始使用。'
        }
      }
    },
    faq: {
      eyebrow: '常见问题',
      title: '开始之前，你可能想了解',
      description: '具体可用能力、价格与状态请以登录后的控制台实时信息为准。',
      items: {
        start: {
          question: '如何开始使用？',
          answer: '注册并进入控制台，创建访问凭证后，按照使用文档完成服务地址配置。'
        },
        capabilities: {
          question: '平台提供哪些能力？',
          answer: '可用范围包括对话与推理、代码协作、图像创作、视频创作、搜索与工具等，具体内容以控制台实时展示为准。'
        },
        usage: {
          question: '在哪里查看用量与费用？',
          answer: '登录控制台后可查看余额、请求记录、使用明细和费用信息。'
        },
        failure: {
          question: '遇到请求失败时如何排查？',
          answer: '先查看服务状态和请求记录，再根据错误信息核对访问范围、额度与客户端配置。'
        }
      }
    },
    cta: {
      eyebrow: '现在开始',
      title: '准备好使用统一的智能服务了吗？',
      description: '创建账号并进入控制台，按照使用文档完成首次配置。',
      docsAction: '阅读使用文档'
    },
    footer: {
      tagline: '稳定、清晰、可管理的智能服务入口',
      allRightsReserved: '保留所有权利。'
    }
  },

  // Key Usage Query Page
  keyUsage: {
    title: 'API Key 用量查询',
    subtitle: '输入您的 API Key 以查看实时消费金额与使用状态',
    placeholder: 'sk-ant-mirror-xxxxxxxxxxxx',
    query: '查询',
    querying: '查询中...',
    privacyNote: '您的 Key 仅在浏览器本地处理，不会被存储',
    dateRange: '统计范围:',
    dateRangeToday: '今日',
    dateRange7d: '7 天',
    dateRange30d: '30 天',
    dateRange90d: '90 天',
    dateRangeCustom: '自定义',
    apply: '应用',
    used: '已使用',
    detailInfo: '详细信息',
    tokenStats: 'Token 统计',
    dailyDetail: '按日明细',
    modelStats: '模型用量统计',
    // Table headers
    date: '日期',
    model: '模型',
    requests: '请求数',
    inputTokens: '输入 Tokens',
    outputTokens: '输出 Tokens',
    cacheCreationTokens: '缓存创建',
    cacheReadTokens: '缓存读取',
    cacheWriteTokens: '缓存写入',
    totalTokens: '总 Tokens',
    cost: '费用',
    // Status
    quotaMode: 'Key 限额模式',
    walletBalance: '钱包余额',
    // Ring card titles
    totalQuota: '总额度',
    limit5h: '5 小时限额',
    limitDaily: '日限额',
    limit7d: '7 天限额',
    limitWeekly: '周限额',
    limitMonthly: '月限额',
    // Detail rows
    remainingQuota: '剩余额度',
    expiresAt: '过期时间',
    todayExpires: '(今日到期)',
    daysLeft: '({days} 天)',
    usedQuota: '已用额度',
    resetNow: '即将重置',
    subscriptionType: '订阅类型',
    subscriptionExpires: '订阅到期',
    // Usage stat cells
    todayRequests: '今日请求',
    todayInputTokens: '今日输入',
    todayOutputTokens: '今日输出',
    todayTokens: '今日 Tokens',
    todayCacheCreation: '今日缓存创建',
    todayCacheRead: '今日缓存读取',
    todayCost: '今日费用',
    rpmTpm: 'RPM / TPM',
    totalRequests: '累计请求',
    totalInputTokens: '累计输入',
    totalOutputTokens: '累计输出',
    totalTokensLabel: '累计 Tokens',
    totalCacheCreation: '累计缓存创建',
    totalCacheRead: '累计缓存读取',
    totalCost: '累计费用',
    avgDuration: '平均耗时',
    // Messages
    enterApiKey: '请输入 API Key',
    querySuccess: '查询成功',
    queryFailed: '查询失败',
    queryFailedRetry: '查询失败，请稍后重试',
    noDailyUsage: '暂无按日用量数据',
  },

  // Setup Wizard
  setup: {
    title: 'Sub2API 安装向导',
    description: '配置您的 Sub2API 实例',
    database: {
      title: '数据库配置',
      description: '连接到您的 PostgreSQL 数据库',
      host: '主机',
      port: '端口',
      username: '用户名',
      password: '密码',
      databaseName: '数据库名称',
      sslMode: 'SSL 模式',
      passwordPlaceholder: '密码',
      ssl: {
        disable: '禁用',
        require: '要求',
        verifyCa: '验证 CA',
        verifyFull: '完全验证'
      }
    },
    redis: {
      title: 'Redis 配置',
      description: '连接到您的 Redis 服务器',
      host: '主机',
      port: '端口',
      username: '用户名（可选）',
      password: '密码（可选）',
      database: '数据库',
      usernamePlaceholder: '默认用户留空',
      passwordPlaceholder: '密码',
      enableTls: '启用 TLS',
      enableTlsHint: '连接 Redis 时使用 TLS（公共 CA 证书）'
    },
    admin: {
      title: '管理员账户',
      description: '创建您的管理员账户',
      email: '邮箱',
      password: '密码',
      confirmPassword: '确认密码',
      passwordPlaceholder: '至少 8 个字符',
      confirmPasswordPlaceholder: '确认密码',
      passwordMismatch: '密码不匹配'
    },
    ready: {
      title: '准备安装',
      description: '检查您的配置并完成安装',
      database: '数据库',
      redis: 'Redis',
      adminEmail: '管理员邮箱'
    },
    status: {
      testing: '测试中...',
      success: '连接成功',
      testConnection: '测试连接',
      installing: '安装中...',
      completeInstallation: '完成安装',
      completed: '安装完成！',
      redirecting: '正在跳转到登录页面...',
      restarting: '服务正在重启，请稍候...',
      timeout: '服务重启时间超出预期，请手动刷新页面。'
    }
  },

  // Common
}
