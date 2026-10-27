import axios from 'axios'
import { ElMessage } from 'element-plus'
import router from './router'

const api = axios.create({ baseURL: '/api', timeout: 60000 })

api.interceptors.response.use(
  (resp) => resp,
  (err) => {
    const resp = err.response
    if (resp && resp.status === 401 && !resp.config.url.includes('/login')) {
      router.push('/login')
      return Promise.reject(err)
    }
    const msg = (resp && resp.data && resp.data.error) || err.message || '请求失败'
    ElMessage.error(msg)
    return Promise.reject(err)
  }
)

export default api
