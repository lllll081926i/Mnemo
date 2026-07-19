<script setup lang="ts">
import message from '../utils/message'
import UserDAL, { UserTokenMap } from '../user/userdal'
import { ITokenInfo } from '../store'
import Db from '../utils/db'
import fs from 'node:fs'
import path from 'path'
import { decodeName, encodeName } from '../module/flow-enc/utils'
import { localPwd } from '../utils/aria2c'

const handlerAccountImport = () => {
  window.WebShowOpenDialogSync(
    {
      title: '选择需要导入的账户文件',
      buttonLabel: '导入选中的账户文件',
      filters: [{ name: 'user.db', extensions: ['db'] }],
      properties: ['openFile', 'multiSelections', 'showHiddenFiles', 'noResolveAliases', 'treatPackageAsDirectory', 'dontAddToRecent']
    },
    async (files: string[] | undefined) => {
      if (files && files.length > 0) {
        try {
          // 获取内容
          let userList: ITokenInfo[] = []
          let uniqueUserIds = new Set()
          for (let filePath of files) {
            let readData = fs.readFileSync(filePath, 'utf-8')
            let parsedData: any = JSON.parse(<string>decodeName(localPwd, 'aesctr', readData))
            if (Array.isArray(parsedData) && parsedData.every((item) => item.hasOwnProperty('access_token'))) {
              let filteredData: ITokenInfo[] = parsedData.filter((item: ITokenInfo) => {
                if (!uniqueUserIds.has(item.user_id)) {
                  uniqueUserIds.add(item.user_id)
                  return true
                }
                return false
              })
              userList.push(...filteredData)
            }
          }
          if (userList.length > 0) {
            // 设置UserTokenMap
            for (let token of userList) {
              if (token.user_id) {
                UserTokenMap.set(token.user_id, token)
              }
            }
            // 导入到数据库
            Db.saveUserBatch(userList)
              .then(() => {
                window.WinMsgToUpload({ cmd: 'ClearUserToken' })
                window.WinMsgToDownload({ cmd: 'ClearUserToken' })
              })
              .catch()
            await UserDAL.UserLogin(userList[0])
            message.success('导入用户账户数据成功')
          } else {
            message.error('数据错误，导入用户账户数据失败')
          }
        } catch (err) {
          message.error('数据错误，导入用户账户数据失败')
        }
      }
    }
  )
}

const handlerAccountExport = () => {
  if (window.WebShowOpenDialogSync) {
    window.WebShowOpenDialogSync(
      {
        title: '选择一个文件夹，保存导出的数据',
        buttonLabel: '选择',
        properties: ['openDirectory', 'createDirectory']
      },
      (result: string[] | undefined) => {
        if (result && result[0]) {
          let exportFile = path.join(result[0], 'user.db')
          let userList = JSON.stringify(UserDAL.GetUserList())
          let data = encodeName(localPwd, 'aesctr', userList)
          fs.writeFileSync(exportFile, data)
          message.success('导出所有用户账户数据成功')
        }
      }
    )
  }
}
</script>

<template>
  <div class="ui-plain-list">
    <div class="ui-plain-row">
      <span class="ui-plain-label">账号数据</span>
      <div class="ui-plain-control">
        <a-button type="outline" size="small" status="danger" tabindex="-1" @click="handlerAccountExport">导出</a-button>
        <a-button type="outline" size="small" status="success" tabindex="-1" @click="handlerAccountImport">导入</a-button>
      </div>
    </div>
  </div>
</template>
