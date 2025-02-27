# 管理设置面板

GoToSocial 管理设置面板使用 [管理 API](https://docs.gotosocial.org/zh-cn/latest/api/swagger/#operations-tag-admin) 来管理你的实例。它与 [用户设置面板](../user_guide/settings.md) 结合使用，并采用与普通客户端相同的 OAuth 机制（范围：admin）。

## 设置管理员账户权限和登录

要使用管理设置面板，你的账户必须被提升为管理员：

```bash
./gotosocial --config-path ./config.yaml admin account promote --username 你的用户名
```

为了使提权生效，可能需要在运行命令后重启你的实例。

之后，你可以访问 `https://[your-instance-name.org]/settings`，在登录字段中输入你的域名，然后像使用其他客户端一样登录。现在，你应该可以看到管理设置。

## 管理

实例管理设置。

### 举报

![一个展示未解决举报的举报列表。](../public/admin-settings-reports.png)

举报部分显示来自本站用户或外站（匿名显示，仅显示实例名称，不显示具体用户名）的举报列表。

点击举报可以查看其是否已解决（若有理由则显示），更多信息，以及由举报用户选定的被举报贴文列表。你也可以在此视图中将举报标记为已解决，并填写评论。如果该用户来自你的实例，你在此处输入的任何评论都会对创建举报的用户可见。

![待处理的举报的详细视图，显示被举报的贴文和举报理由。](../public/admin-settings-report-detail.png)

点击被举报账户的用户名会在“账户”视图中打开该账户，从而允许你对其执行管理操作。

### 账户

你可以使用此部分搜索账户并对其执行管理操作。

### 域名权限

![已封禁实例列表，有一个字段用于过滤/添加新的屏蔽。下面是批量导入/导出界面的链接](../public/admin-settings-federation.png)

在域名权限部分，你可以创建、删除和查看域名阻止条目、域名允许条目、草稿、排除项和订阅。

关于联合设置的更多详细信息，特别是域名允许和域名屏蔽如何结合使用，请参阅 [联合模式部分](./federation_modes.md) 和 [域名屏蔽部分](./domain_blocks.md)。

#### 域名屏蔽

你可以在搜索字段中输入一个要封禁的域名，这将过滤列表以显示你是否已有该域名的屏蔽条目。

点击“封禁”会显示一个表单，允许你添加公开和/或私人评论，并提交以添加屏蔽。

添加封禁后，该实例上的所有已知账户将被封禁，并阻止与该被屏蔽实例上的任何用户的新互动。

#### 域名允许

域名允许部分的工作方式与域名屏蔽部分类似，只是用于明确的域名允许而不是域名屏蔽。

#### 导入/导出

你可以在这一部分批量导入/导出JSON、CSV或纯文本格式的域名权限条目。

![导入中包含的域列表，提供选择某些或全部域的方法，更改其域，以及更新子域使用方法。](../public/admin-settings-federation-import-export.png)

通过输入字段或文件导入列表后，你可以在导入子集之前查看列表中的条目。你还会在使用子域的条目中收到警告，此处还提供一种轻松将其更改为主域的方法。

#### 草稿

在这一部分，你可以创建、搜索、接受和拒绝域名权限草稿。

域名权限草稿是已被提议，但尚未生效的域域名权限条目（可以手动创建或从已订阅的阻止/允许列表中添加）。

在接受前，域名权限草稿将对目标域名的联合没有任何影响。一旦被接受，它将被转换为域名阻止条目或域名允许条目，并开始执行。

#### 例外

在这一部分，您可以创建、搜索和移除域名权限例外条目。

域名权限例外可以防止某域名（及其所有子域）的权限被域名权限订阅自动管理。

例如，如果你为域名 `example.org` 创建例外条目，那么在创建域名权限草稿和域名阻止/允许条目时，阻止列表或允许列表订阅将排除 `example.org` 及其任何子域（如 `sub.example.org`,`another.sub.example.org` 等）的条目。

此功能可以让你在明确知道是否要与某个域名进行联合的情况下，手动管理被设为例外的域名的权限，不受域名权限订阅中包含的条目的影响。

请注意，仅针对某个域名创建排除条目本身并不会对与该域名的联合产生影响，它只有与权限订阅结合使用时才会发挥作用。

#### 订阅

在这一部分，你可以创建、搜索、编辑、测试和移除域名权限订阅。

域名权限订阅允许您指定权限列表的托管地址。默认情况下，每天晚上11点，你的实例将获取并解析订阅的每个列表，并根据列表中的条目创建域名权限（或域名权限草稿）。

##### 标题

您可以选择使用标题字段为订阅设置标题，以便对自己和其他管理员进行提醒。

例如，您可能会订阅 `https://lists.example.org/baddies.csv` 上的列表，并将该订阅的标题设置为某些反映该列表内容的描述，如“基础阻止列表（最为恶劣的实例）”或类似描述。

##### 订阅优先级

当你指定了多个域名权限订阅时，它们将按优先级顺序从最高优先级 (255) 到最低优先级 (0) 被获取和解析。

在优先级排名靠前的列表中发现的权限将覆盖在优先级排名靠后的列表中的权限。

有关优先级的更多信息，参见单独的[域名权限订阅](./domain_permission_subscriptions.md)文档。

##### 权限类型

你可以使用此下拉菜单选择为在订阅地址中发现的权限创建的条目类型，可以为阻止或允许。

##### 内容类型

您可以使用此下拉菜单选择订阅地址指向的列表的内容类型。

要订阅与 Mastodon 格式兼容的权限列表，可以选择 CSV，要使用纯文本域名列表，可以选择 plain，也可以选择 JSON，用于订阅以 JSON 格式导出的列表。

##### 基础认证（Basic Auth）

勾选此复选框，可以为订阅列表提供基础认证用户名和/或密码凭证，这些凭证将在每次向订阅地址请求列表时一并发送。

##### 接管孤立权限条目

如果勾选此框，那么在以下情况下，任何现有的域名权限将由该订阅管理:

1. 该权限条目没有关联的订阅 ID（即，它们不受任何域权限订阅管理）。
2. 该权限条目与此订阅地址中包含的域名权限匹配。

有关孤立权限的更多信息，参见单独的[域名权限订阅](./domain_permission_subscriptions.md)文档。

##### 将此条目设为草稿

勾选此复选框后（该复选框默认勾选），通过此订阅创建的任何权限条目将以**草稿**类型创建，需要手动批准才能生效。

建议保留此复选框为已勾选状态，除非您完全信任订阅列表，以避免无意中阻止或允许您不想阻止或允许的域。

##### 测试订阅

要测试订阅是否可以被成功解析，首先创建订阅，然后在该订阅的详情视图中，点击“测试”按钮。

如果您的实例能够获取并解析订阅地址处的权限列表，则在点击“测试”后您将看到这些权限的列表。否则，您将看到一条错误信息。

![订阅详情视图的截图，箭头指向靠近底部的测试部分。](../public/admin-settings-federation-subscription-test.png)

## 管理

实例管理设置。

### 操作

运行一次性管理操作。

#### 电子邮件

你可以使用此部分向指定的电子邮件地址发送测试邮件，并附加可选的测试信息。

#### 媒体

你可以使用此部分运行清理外站媒体缓存的操作，可以指定天数。超过指定天数的媒体将从存储中删除（s3 或本地）。以这种方式删除的媒体将未来需要时重新尝试获取。此操作在功能上与自动运行的媒体清理相同。

#### 密钥

你可以使用此部分使来自特定外站实例的公钥过期/失效。下次你的实例收到使用过期密钥的签名请求时，它将尝试重新获取和存储公钥。

### 自定义表情

包含在外站贴文中的自定义表情将自动获取，但要在你的帖子中使用它们，必须在你的实例上启用。

#### 本站

![本站自定义表情部分，显示按类别排序的自定义表情概览。有很多加菲猫表情。](../public/admin-settings-emoji-local.png)

此部分显示你的实例上启用的所有自定义表情的概览，按类别排序。点击某个表情可显示其详细信息，并提供更改类别或图像的选项，或完全删除它。这里无法更新短代码，你需要自己上传带有新短代码的表情（可以选择删除旧的表情）。

在概览下方，你可以在预览表情在贴文中的效果后上传自己的自定义表情。支持 PNG 和（动画）GIF 格式。

#### 外站

![外站自定义表情部分，显示从输入的贴文中解析的 3 个表情的列表： blobcat、blobfoxbox 和 blobhajmlem。可以选择它们，微调短代码，并在提交复制或删除操作前为其分配类别](../public/admin-settings-emoji-remote.png)

通过“外站”部分，你可以查找任何外站贴文的链接（前提是该实例未被封禁）。如果使用了任何自定义表情，它们将被列出，这样就提供了一种轻松复制到本站表情的方法（供你自己在贴文中使用），或者也可以禁止它们（从贴文中隐藏）。

**注意：**由于 testrig 服务器未进行联合，此功能在开发过程中无法使用（500：内部服务器错误）。

### 实例设置

![GoToSocial 管理面板的截图，显示了更改实例设置的字段](../public/admin-settings-instance.png)

在这里，你可以为你的实例设置各种元数据，如显示名称/标题、缩略图、（简短）描述和联系信息。

#### 实例外观

这些设置主要影响你的实例在网络和他人眼中的显示方式。

你的 **实例标题** 将显示在你实例每个网页的顶部，并在 OpenGraph 元标签中出现，所以选择一个能代表你实例氛围的名称。

**实例头像** 类似于你实例的吉祥物。它将出现在每个网页顶上的实例标题旁边，并作为浏览器标签、OpenGraph 链接等的预览图像。

如果你设置了实例头像，我们强烈建议同时设置 **头像描述**。这将为你设置为头像的图片提供替代文字，帮助屏幕阅读器用户理解图片中描绘的内容。替代文本应保持简短明了。

#### 实例描述

你可以使用这些字段设置实例的简短和完整描述，并为当前和潜在用户提供实例使用条款。

**简短描述** 将显示在实例主页的顶部附近，以及响应 `/api/v1/instance` 查询时显示。

可以提供一些精辟的内容，以便访问你的实例的访客对你的实例有一个第一印象。例如：

> 这是一个 ACG 爱好者的实例！
>
> 不管磕什么都可以来注册。

或者：

> 这是一个单用户实例，只属于我！
>
> 这是我的主页：@your_username

**完整描述** 将显示在你的实例的 /about 页面上，并在响应 `/api/v1/instance` 查询时显示。

你可以用它来提供如下信息：

- 你的实例的历史、理念、态度和氛围
- 你实例上的居民倾向于发布的内容类型
- 如何在你的实例上获得账户（如果可能的话）
- 一个拥有账户的用户列表，希望更容易被找到

**使用条款** 框也会出现在你的实例的 /about 页面上，并在响应 `/api/v1/instance` 查询时显示。

用它来填写如下内容：

- 法律术语（版权、GDPR 或相关链接）
- 联合政策
- 数据政策
- 账户删除/封禁政策

以上所有字段都接受 **markdown** 输入，因此你可以编写合适的列表、代码块、水平线、引用块或任何你喜欢的内容。

你也可以使用标准 `@user[@domain]` 格式提及账户。

查看 [markdown 速查表](https://markdownguide.offshoot.io/cheat-sheet/) 以了解可以做些什么。

### 实例联系信息

在此部分中，你可以向访问你实例的用户提供一种方便的方法，以联系你的实例管理员。

设置好的联系人账户和/或电子邮件地址的链接将出现在实例的每个网页底部、/about 页面的“联系”部分，以及响应 `/api/v1/instance` 查询时显示。

选择的 **联系人用户** 必须是实例上的活跃（未封禁）的管理员和/或站务。

如果你是在单用户实例上并将管理员权限授予你的主账户，你只需在此处填写自己的用户名即可；无需为此专门创建管理账户。

### 实例自定义 CSS

自定义 CSS 允许您进一步调整通过浏览器访问的实例时的外观。

这些自定义 CSS 将应用于实例的所有页面。但用户主题和 CSS 仍优先于此处的自定义设置。

有关为您的实例编写自定义 CSS 的一些技巧，请参阅[自定义 CSS](../user_guide/custom_css.md)页面。
