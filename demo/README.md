# pipet Demo 测试用例示例

本目录包含各种常见的接口测试用例示例，覆盖多种入参方式和测试场景。

## 文件说明

| 文件 | 说明 | 覆盖场景 |
|------|------|----------|
| `http_methods.psv` | HTTP方法测试 | GET、POST、PUT、PATCH、DELETE |
| `json_body.psv` | JSON请求体测试 | 简单对象、嵌套对象、数组、特殊字符 |
| `form_data.psv` | 表单数据测试 | URL编码、中文、多值字段、空值 |
| `file_upload.psv` | 文件上传测试 | 单文件、多文件、文件+表单 |
| `headers_params.psv` | 请求头和URL参数 | Bearer认证、自定义头、查询参数 |
| `assert_extract.psv` | 断言和变量提取 | JSON断言、正则断言、变量提取 |
| `streaming.psv` | 流式接口测试 | SSE、OpenAI风格流式响应 |
| `combined.psv` | 组合测试 | 多种入参方式组合、CRUD流程 |

## 使用方法

```bash
# 运行所有demo测试用例
pipet ./demo

# 运行特定文件
pipet ./demo/http_methods.psv

# 使用标签过滤
pipet ./demo -t smoke
pipet ./demo -t api,json
```

## 测试用例字段说明

| 字段 | 说明 | 示例 |
|------|------|------|
| `id` | 测试用例唯一标识 | `get_01` |
| `skip` | 是否跳过（0/1） | `0` |
| `desc` | 测试用例描述 | `GET请求-获取用户列表` |
| `method` | HTTP方法 | `GET`/`POST`/`PUT`/`DELETE` |
| `url` | 请求URL | `{{base_url}}/api/users` |
| `headers` | 请求头（JSON格式） | `{"Authorization":"Bearer {{token}}"}` |
| `params` | URL参数（&分隔） | `page=1&limit=10` |
| `json` | JSON请求体 | `{"name":"张三"}` |
| `form` | 表单数据（&分隔） | `username=test&password=123` |
| `expected_status` | 期望状态码 | `200` |
| `expected_body` | 期望响应体 | `{"code":200}` |
| `extract` | 变量提取规则 | `token=$.data.token` |
| `tags` | 标签（逗号分隔） | `api,smoke` |
| `stream_mode` | 流式模式（0/1） | `1` |
| `stream_assert` | 流式断言规则 | `[{"kind":"contains","pattern":"success"}]` |

## 变量替换

支持使用 `{{var_name}}` 格式引用全局变量：

```yaml
# config/config.yaml
target:
  base_url: "https://api.example.com"
```

```psv
id|url
test_01|{{base_url}}/api/users
```

## 文件上传

支持两种文件路径格式：
- `@./path/to/file.txt` - 相对路径
- `file:///absolute/path/to/file.txt` - 绝对路径

```psv
id|form
upload_01|file=@./data/test.txt
```

## 变量提取

使用 JSONPath 提取响应体中的变量：

```psv
id|extract
test_01|token=$.data.token&user_id=$.data.user.id
```

提取后的变量可在后续测试用例中使用：`{{token}}`、`{{user_id}}`