document.addEventListener('DOMContentLoaded', () => {
  // --- View and Navigation Elements ---
  const mainMenu = document.getElementById('main-menu-view')
  const serviceAccessView = document.getElementById('service-access-view')
  const externalAccessView = document.getElementById('external-access-view')
  const accessRequestHubView = document.getElementById(
    'access-request-hub-view',
  )

  const gotoServiceAccessCard = document.getElementById(
    'goto-service-access-card',
  )
  const gotoExternalAccessCard = document.getElementById(
    'goto-external-access-card',
  )
  const gotoHubCard = document.getElementById('goto-hub-card')

  const backToMenuBtnSvc = document.getElementById('back-to-menu-btn-svc')
  const backToMenuBtnExt = document.getElementById('back-to-menu-btn-ext')
  const backToMenuBtnHub = document.getElementById('back-to-menu-btn-hub')

  // --- Service Access Tool Elements ---
  const caForm = document.getElementById('cluster-access-form')
  const sourceSvcSelect = document.getElementById('ca-source-svc')
  const targetSvcSelect = document.getElementById('ca-target-svc')
  const caDurationSelect = document.getElementById('ca-duration')
  const directionSelect = document.getElementById('ca-direction')
  const portsInput = document.getElementById('ca-ports')
  const sourceNsFilter = document.getElementById('ca-source-ns-filter')
  const targetNsFilter = document.getElementById('ca-target-ns-filter')
  const refreshServicesBtn = document.getElementById('refresh-services-btn')
  const refreshAccessBtnSvc = document.getElementById('refresh-access-btn-svc')
  const caCreateBtn = document.getElementById('ca-create-btn')
  const caSubmitReviewBtn = document.getElementById('ca-submit-review-btn')
  const caDescriptionInput = document.getElementById('ca-description')

  // --- External Access Tool Elements ---
  const eaForm = document.getElementById('external-access-form')
  const eaCidrInput = document.getElementById('ea-cidr')
  const eaNsFilter = document.getElementById('ea-ns-filter')
  const eaServiceSelect = document.getElementById('ea-service')
  const eaDirectionSelect = document.getElementById('ea-direction')
  const eaDurationSelect = document.getElementById('ea-duration')
  const eaPortsInput = document.getElementById('ea-ports')
  const refreshEaServicesBtn = document.getElementById(
    'refresh-ea-services-btn',
  )
  const refreshAccessBtnExt = document.getElementById('refresh-access-btn-ext')
  const eaCreateBtn = document.getElementById('ea-create-btn')
  const eaSubmitReviewBtn = document.getElementById('ea-submit-review-btn')
  const eaDescriptionInput = document.getElementById('ea-description')

  // --- Hub Elements ---
  const pendingRequestsList = document.getElementById('pending-requests-list')
  const noPendingRequestsMessage = document.getElementById(
    'no-pending-requests-message',
  )

  // --- Shared and Dashboard Elements ---
  const logDashboard = document.getElementById('results-log-dashboard')
  const accessListDashboard = document.getElementById(
    'active-access-list-dashboard',
  )
  const refreshAccessBtnDashboard = document.getElementById(
    'refresh-access-btn-dashboard',
  )
  const logSvc = document.getElementById('results-log-svc')
  const accessListSvc = document.getElementById('active-access-list-svc')
  const logExt = document.getElementById('results-log-ext')
  const accessListExt = document.getElementById('active-access-list-ext')

  if (!mainMenu || !serviceAccessView || !externalAccessView) return

  let allServices = []
  let serviceViewInitialized = false
  let externalViewInitialized = false

  const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const socket = new WebSocket(`${wsProtocol}//${window.location.host}/ws`)

  // --- Log Rendering Functions ---
  const renderLogEntry = (entry) => {
    const newLog = document.createElement('div')
    newLog.textContent = entry.payload
    newLog.className = entry.className || 'log-info'

    logDashboard.appendChild(newLog.cloneNode(true))
    logDashboard.scrollTop = logDashboard.scrollHeight

    if (entry.logType === 'Service') {
      logSvc.appendChild(newLog.cloneNode(true))
      logSvc.scrollTop = logSvc.scrollHeight
    } else if (entry.logType === 'External') {
      logExt.appendChild(newLog.cloneNode(true))
      logExt.scrollTop = logExt.scrollHeight
    }
  }

  const fetchAndRenderLogs = async () => {
    try {
      const response = await fetch('/api/logs')
      if (!response.ok) throw new Error('Failed to fetch logs')
      const logs = await response.json()

      logDashboard.innerHTML = ''
      logSvc.innerHTML = ''
      logExt.innerHTML = ''

      logs.forEach(renderLogEntry)
    } catch (error) {
      console.error('Error fetching activity log:', error)
      renderLogEntry({
        payload: 'Could not load historical activity log.',
        className: 'log-error',
        logType: 'Global',
      })
    }
  }

  // --- View Management ---
  const showView = (viewId) => {
    document.querySelectorAll('.view').forEach((view) => {
      view.style.display = 'none'
    })
    const viewToShow = document.getElementById(viewId)
    if (viewToShow) {
      viewToShow.style.display = 'block'
    }
  }

  // --- Shared Functions ---
  const fetchAllServices = async () => {
    try {
      const response = await fetch('/api/services')
      if (!response.ok) throw new Error('Failed to fetch services')
      allServices = await response.json()
    } catch (error) {
      console.error('Error fetching master service list:', error)
      renderLogEntry({
        payload: 'Could not load services from the cluster.',
        className: 'log-error',
        logType: 'Global',
      })
    }
  }

  const renderAccessList = (container, accessData) => {
    container.innerHTML = ''
    if (!accessData || accessData.length === 0) {
      container.innerHTML =
        '<p>No active access policies of this type found.</p>'
      return
    }
    const table = document.createElement('table')
    table.innerHTML = `<thead><tr><th style="width: 60%;">Access Details</th><th>Expires</th><th>Action</th></tr></thead><tbody></tbody>`
    const tbody = table.querySelector('tbody')
    accessData.forEach((access) => {
      const expires =
        access.expiresAt === -1
          ? 'Infinite'
          : new Date(access.expiresAt * 1000).toLocaleTimeString()

      let directionArrow = ''
      let detailsHtml = ''
      const portsDisplay = access.ports || 'Default'
      const rowClass = access.status === 'Pending' ? 'access-pending' : ''

      if (access.type === 'Service') {
        switch (access.direction) {
          case 'ingress':
            directionArrow = '←'
            break
          case 'egress':
            directionArrow = '→'
            break
          case 'all':
          case 'both':
            directionArrow = '↔'
            break
          default:
            directionArrow = '?'
        }
        detailsHtml = `
                    <div style="font-weight: 500; word-break: break-all; display: flex; align-items: center; gap: 8px;">
                        <span>${access.source}</span>
                        <span style="color: var(--md-sys-color-primary); font-size: 1.2em;">${directionArrow}</span>
                        <span>${access.target}</span>
                    </div>
                    <div style="color: #5f6368; font-size: 0.8rem; margin-top: 4px;">Ports: ${portsDisplay}</div>`
      } else {
        switch (access.direction) {
          case 'ingress':
            directionArrow = '→'
            break
          case 'egress':
            directionArrow = '←'
            break
          case 'all':
          case 'both':
            directionArrow = '↔'
            break
          default:
            directionArrow = '?'
        }
        detailsHtml = `
                    <div style="font-weight: 500; word-break: break-all; display: flex; align-items: center; gap: 8px;">
                        <span class="material-symbols-outlined" style="font-size: 1.2em;">public</span>
                        <span>${access.source}</span>
                        <span style="color: var(--md-sys-color-primary); font-size: 1.2em;">${directionArrow}</span>
                        <span>${access.target}</span>
                    </div>
                    <div style="color: #5f6368; font-size: 0.8rem; margin-top: 4px;">Ports: ${portsDisplay}</div>`
      }

      let actionButtonHtml = ''
      if (access.status === 'Pending') {
        actionButtonHtml = `<button class="btn btn-filled btn-small view-request-btn">View Request</button>`
      } else {
        actionButtonHtml = `<button class="btn btn-filled btn-small revoke-btn" data-type="${access.type}" data-name="${access.name}" data-namespace="${access.namespace}">Revoke</button>`
      }

      const row = document.createElement('tr')
      if (rowClass) {
        row.classList.add(rowClass)
      }
      row.innerHTML = `
                <td>${detailsHtml}</td>
                <td>${expires}</td>
                <td>${actionButtonHtml}</td>`
      tbody.appendChild(row)
    })
    container.appendChild(table)
  }

  const fetchAndDisplayActiveAccesses = async () => {
    const buttons = [
      refreshAccessBtnDashboard,
      refreshAccessBtnSvc,
      refreshAccessBtnExt,
    ]
    buttons.forEach((btn) => (btn.disabled = true))
    ;[accessListDashboard, accessListSvc, accessListExt].forEach(
      (el) => (el.innerHTML = '<p>Refreshing...</p>'),
    )

    try {
      const response = await fetch('/api/active-accesses')
      if (!response.ok) throw new Error('Failed to fetch active accesses')
      const allActiveAccesses = await response.json()

      renderAccessList(accessListDashboard, allActiveAccesses)
      renderAccessList(
        accessListSvc,
        allActiveAccesses.filter((a) => a.type === 'Service'),
      )
      renderAccessList(
        accessListExt,
        allActiveAccesses.filter((a) => a.type === 'External'),
      )
    } catch (error) {
      console.error('Error fetching active accesses:', error)
      ;[accessListDashboard, accessListSvc, accessListExt].forEach(
        (el) =>
          (el.innerHTML =
            '<p class="log-error">Could not load active accesses.</p>'),
      )
    } finally {
      buttons.forEach((btn) => (btn.disabled = false))
    }
  }

  const fetchAndRenderPendingRequests = async () => {
    pendingRequestsList.innerHTML = '<p>Refreshing pending requests...</p>'
    pendingRequestsList.style.display = 'block'
    noPendingRequestsMessage.style.display = 'none'

    try {
      const response = await fetch('/api/pending-requests')
      if (!response.ok) {
        throw new Error('Failed to fetch pending requests')
      }
      const requests = await response.json()

      if (requests && requests.length > 0) {
        pendingRequestsList.style.display = 'block'
        noPendingRequestsMessage.style.display = 'none'
        pendingRequestsList.innerHTML = ''

        const table = document.createElement('table')
        table.innerHTML = `<thead><tr><th>Requestor</th><th>Type</th><th style="width: 35%;">Details</th><th>Duration</th><th>Actions</th></tr></thead><tbody></tbody>`
        const tbody = table.querySelector('tbody')

        requests.forEach((req) => {
          let details = ''
          if (req.requestType === 'Service') {
            details = `<strong>Source:</strong> ${req.sourceService}<br><strong>Target:</strong> ${req.targetService}`
          } else {
            details = `<strong>Source:</strong> ${req.cidr}<br><strong>Target:</strong> ${req.service}`
          }

          let directionText = req.direction
          switch (req.direction) {
            case 'ingress':
              directionText = 'Ingress'
              break
            case 'egress':
              directionText = 'Egress'
              break
            case 'all':
            case 'both':
              directionText = 'All (Ingress & Egress)'
              break
          }
          details += `<br><strong>Direction:</strong> ${directionText}`

          if (req.description) {
            details += `<br><strong>Desc:</strong> ${req.description}`
          }

          let permissionsHtml = ''
          const permissionsList = []

          const getPermsForNs = (ns, resource) => [
            { text: `CREATE services in '${ns}'`, satisfied: false },
            {
              text: `CREATE ${resource}.maxtac.vtk.io in '${ns}'`,
              satisfied: false,
            },
          ]

          if (req.requestType === 'Service') {
            const sourceParts = req.sourceService.split('/')
            const targetParts = req.targetService.split('/')
            if (sourceParts.length === 2 && targetParts.length === 2) {
              const sourcePerms = getPermsForNs(sourceParts[0], 'accesses')
              const targetPerms = getPermsForNs(targetParts[0], 'accesses')

              if (req.status === 'PendingTarget') {
                sourcePerms.forEach((p) => (p.satisfied = true))
              } else if (req.status === 'PendingSource') {
                targetPerms.forEach((p) => (p.satisfied = true))
              }
              permissionsList.push(...sourcePerms, ...targetPerms)
            }
          } else {
            const serviceParts = req.service.split('/')
            if (serviceParts.length === 2) {
              permissionsList.push(
                ...getPermsForNs(serviceParts[0], 'externalaccesses'),
              )
            }
          }

          if (permissionsList.length > 0) {
            const permsAsHtml = permissionsList
              .map(
                (p) =>
                  `<small class="${p.satisfied ? 'perm-satisfied' : 'perm-needed'}">${p.text}</small>`,
              )
              .join('<br>')

            permissionsHtml = `<div style="margin-top: 8px; padding-top: 4px; border-top: 1px solid #eee;">
                                  <strong>Permissions Needed:</strong><br>
                                  ${permsAsHtml}
                                </div>`
          }
          details += permissionsHtml

          let typeAndStatus = req.requestType
          if (req.status && req.status !== 'PendingFull') {
            const friendlyStatus = req.status.replace('Pending', 'Pending ')
            typeAndStatus += `<br><small style="color: var(--log-color-warning);">${friendlyStatus}</small>`
          }

          let actionButtonsHtml = ''
          const isOwner =
            typeof currentUserEmail !== 'undefined' &&
            currentUserEmail === req.requestor

          if (isOwner && req.canSelfApprove) {
            actionButtonsHtml = `
              <div style="display: flex; flex-direction: column; gap: 8px;">
                  <button class="btn btn-filled btn-small approve-btn" data-id="${req.requestID}" style="--md-filled-button-container-height: 32px;">Approve</button>
                  <button class="btn btn-filled btn-small deny-btn" data-id="${req.requestID}" style="--md-filled-button-container-height: 32px;">Abort</button>
              </div>
            `
          } else if (isOwner) {
            actionButtonsHtml = `
              <button class="btn btn-filled btn-small deny-btn" data-id="${req.requestID}" style="--md-filled-button-container-height: 32px;">
                Abort
              </button>
            `
          } else {
            actionButtonsHtml = `
              <div style="display: flex; flex-direction: column; gap: 8px;">
                  <button class="btn btn-filled btn-small approve-btn" data-id="${req.requestID}" style="--md-filled-button-container-height: 32px;">Approve</button>
                  <button class="btn btn-filled btn-small deny-btn" data-id="${req.requestID}" style="--md-filled-button-container-height: 32px;">Deny</button>
              </div>
            `
          }

          const row = document.createElement('tr')
          row.innerHTML = `
                  <td>${req.requestor}</td>
                  <td>${typeAndStatus}</td>
                  <td>${details}<br><small><strong>Ports:</strong> ${req.ports || 'Default'}</small></td>
                  <td>${req.duration > 0 ? `${req.duration / 60} mins` : 'Infinite'}</td>
                  <td>${actionButtonsHtml}</td>
              `
          tbody.appendChild(row)
        })
        pendingRequestsList.appendChild(table)
      } else {
        pendingRequestsList.style.display = 'none'
        noPendingRequestsMessage.style.display = 'flex'
      }
    } catch (error) {
      console.error('Error fetching pending requests:', error)
      pendingRequestsList.style.display = 'block'
      noPendingRequestsMessage.style.display = 'none'
      pendingRequestsList.innerHTML =
        '<p class="log-error">Could not load pending requests.</p>'
    }
  }

  const selectivelyResetCaForm = () => {
    portsInput.value = ''
    caDescriptionInput.value = ''
  }

  const selectivelyResetEaForm = () => {
    eaCidrInput.value = ''
    eaPortsInput.value = ''
    eaDescriptionInput.value = ''
  }

  const updateCaServiceDropdown = (serviceDropdown, namespaceFilter) => {
    serviceDropdown.innerHTML = ''
    const placeholder = document.createElement('option')
    placeholder.value = ''
    placeholder.textContent = 'Select a service...'
    placeholder.disabled = true
    placeholder.selected = true
    serviceDropdown.appendChild(placeholder)
    const filteredServices = allServices.filter(
      (svc) => namespaceFilter === 'all' || svc.namespace === namespaceFilter,
    )
    if (filteredServices.length > 0) {
      filteredServices.forEach((svc) => {
        const option = document.createElement('option')
        option.value = svc.compound
        option.textContent = svc.compound
        serviceDropdown.appendChild(option)
      })
    }
  }

  const initializeCaFilters = async () => {
    refreshServicesBtn.disabled = true
    await fetchAllServices()
    const namespaces = [
      ...new Set(allServices.map((svc) => svc.namespace)),
    ].sort()
    sourceNsFilter.innerHTML = ''
    targetNsFilter.innerHTML = ''
    const allNsOption = document.createElement('option')
    allNsOption.value = 'all'
    allNsOption.textContent = 'All Namespaces'
    sourceNsFilter.appendChild(allNsOption.cloneNode(true))
    targetNsFilter.appendChild(allNsOption)
    namespaces.forEach((ns) => {
      const option = document.createElement('option')
      option.value = ns
      option.textContent = ns
      sourceNsFilter.appendChild(option.cloneNode(true))
      targetNsFilter.appendChild(option)
    })
    updateCaServiceDropdown(sourceSvcSelect, 'all')
    updateCaServiceDropdown(targetSvcSelect, 'all')
    refreshServicesBtn.disabled = false
  }

  const initServiceAccessView = () => {
    if (serviceViewInitialized) return
    initializeCaFilters()
    serviceViewInitialized = true
  }

  const updateEaServiceDropdown = (serviceDropdown, namespaceFilter) => {
    serviceDropdown.innerHTML = ''
    const placeholder = document.createElement('option')
    placeholder.value = ''
    placeholder.textContent = 'Select a service...'
    placeholder.disabled = true
    placeholder.selected = true
    serviceDropdown.appendChild(placeholder)
    const filteredServices = allServices.filter(
      (svc) => namespaceFilter === 'all' || svc.namespace === namespaceFilter,
    )
    if (filteredServices.length > 0) {
      filteredServices.forEach((svc) => {
        const option = document.createElement('option')
        option.value = svc.compound
        option.textContent = svc.compound
        serviceDropdown.appendChild(option)
      })
    }
  }

  const initializeEaFilters = async () => {
    refreshEaServicesBtn.disabled = true
    await fetchAllServices()
    const namespaces = [
      ...new Set(allServices.map((svc) => svc.namespace)),
    ].sort()

    eaNsFilter.innerHTML = ''
    const allNsOption = document.createElement('option')
    allNsOption.value = 'all'
    allNsOption.textContent = 'All Namespaces'
    eaNsFilter.appendChild(allNsOption)

    namespaces.forEach((ns) => {
      const option = document.createElement('option')
      option.value = ns
      option.textContent = ns
      eaNsFilter.appendChild(option)
    })

    updateEaServiceDropdown(eaServiceSelect, 'all')
    refreshEaServicesBtn.disabled = false
  }

  const initExternalAccessView = () => {
    if (externalViewInitialized) return
    initializeEaFilters()
    externalViewInitialized = true
  }

  const initHubView = () => {
    fetchAndRenderPendingRequests()
  }

  // --- WebSocket Handlers ---
  socket.onopen = () => {
    console.log('WebSocket connection established.')
    fetchAndRenderLogs()
    fetchAndDisplayActiveAccesses()
  }

  socket.onclose = () => {
    renderLogEntry({
      payload: 'Connection lost. Please refresh the page.',
      className: 'log-warning',
      logType: 'Global',
    })
  }

  socket.onmessage = (event) => {
    const data = JSON.parse(event.data)
    renderLogEntry(data)

    const isSuccessfulSubmission = data.payload.includes('submitted for review')
    const isApprovalOrDenial =
      data.payload.includes('approved') ||
      data.payload.includes('denied') ||
      data.payload.includes('aborted')
    const isCreationComplete = data.type === 'applyComplete'
    const isRevocationInitiated = data.payload.includes('Revocation initiated')

    if (isCreationComplete || isApprovalOrDenial || isSuccessfulSubmission) {
      fetchAndDisplayActiveAccesses()
      if (accessRequestHubView.style.display === 'block') {
        fetchAndRenderPendingRequests()
      }
    }

    if (isRevocationInitiated) {
      setTimeout(() => {
        fetchAndDisplayActiveAccesses()
      }, 3000)
    }

    if (isSuccessfulSubmission) {
      if (
        serviceAccessView.style.display === 'block' &&
        (data.logType === 'Service' || data.logType === 'Request')
      ) {
        selectivelyResetCaForm()
      }
      if (
        externalAccessView.style.display === 'block' &&
        (data.logType === 'External' || data.logType === 'Request')
      ) {
        selectivelyResetEaForm()
      }
    }
  }

  // --- Navigation ---
  gotoServiceAccessCard.addEventListener('click', () => {
    showView('service-access-view')
    initServiceAccessView()
  })
  gotoExternalAccessCard.addEventListener('click', () => {
    showView('external-access-view')
    initExternalAccessView()
  })
  gotoHubCard.addEventListener('click', () => {
    showView('access-request-hub-view')
    initHubView()
  })
  backToMenuBtnSvc.addEventListener('click', (e) => {
    e.preventDefault()
    showView('main-menu-view')
  })
  backToMenuBtnExt.addEventListener('click', (e) => {
    e.preventDefault()
    showView('main-menu-view')
  })
  backToMenuBtnHub.addEventListener('click', (e) => {
    e.preventDefault()
    showView('main-menu-view')
  })

  // --- Form Actions ---
  caCreateBtn.addEventListener('click', () => {
    if (!caForm.checkValidity()) {
      caForm.reportValidity()
      return
    }
    socket.send(
      JSON.stringify({
        command: 'requestClusterAccess',
        sourceService: sourceSvcSelect.value,
        targetService: targetSvcSelect.value,
        duration: parseInt(caDurationSelect.value, 10),
        direction: directionSelect.value,
        ports: portsInput.value,
      }),
    )
  })

  caSubmitReviewBtn.addEventListener('click', () => {
    if (!caForm.checkValidity()) {
      caForm.reportValidity()
      return
    }
    socket.send(
      JSON.stringify({
        command: 'submitAccessRequest',
        sourceService: sourceSvcSelect.value,
        targetService: targetSvcSelect.value,
        duration: parseInt(caDurationSelect.value, 10),
        direction: directionSelect.value,
        ports: portsInput.value,
        description: caDescriptionInput.value,
      }),
    )
  })

  eaCreateBtn.addEventListener('click', () => {
    if (!eaForm.checkValidity()) {
      eaForm.reportValidity()
      return
    }
    socket.send(
      JSON.stringify({
        command: 'requestExternalAccess',
        cidr: eaCidrInput.value,
        service: eaServiceSelect.value,
        duration: parseInt(eaDurationSelect.value, 10),
        direction: eaDirectionSelect.value,
        ports: eaPortsInput.value,
      }),
    )
  })

  eaSubmitReviewBtn.addEventListener('click', () => {
    if (!eaForm.checkValidity()) {
      eaForm.reportValidity()
      return
    }
    socket.send(
      JSON.stringify({
        command: 'submitAccessRequest',
        cidr: eaCidrInput.value,
        service: eaServiceSelect.value,
        duration: parseInt(eaDurationSelect.value, 10),
        direction: eaDirectionSelect.value,
        ports: eaPortsInput.value,
        description: eaDescriptionInput.value,
      }),
    )
  })

  // --- Other Event Listeners ---
  document.body.addEventListener('click', (event) => {
    const revokeBtn = event.target.closest('.revoke-btn')
    if (revokeBtn) {
      if (confirm('Are you sure you want to revoke this access?')) {
        const row = revokeBtn.closest('tr')
        if (row) {
          row.style.opacity = '0.5'
          row.style.pointerEvents = 'none'
        }
        revokeBtn.disabled = true

        socket.send(
          JSON.stringify({
            command:
              revokeBtn.dataset.type === 'Service'
                ? 'revokeClusterAccess'
                : 'revokeExternalAccess',
            name: revokeBtn.dataset.name,
            namespace: revokeBtn.dataset.namespace,
          }),
        )
      }
    }

    const viewRequestBtn = event.target.closest('.view-request-btn')
    if (viewRequestBtn) {
      showView('access-request-hub-view')
      initHubView()
    }

    const approveBtn = event.target.closest('.approve-btn')
    if (approveBtn) {
      if (confirm('Are you sure you want to approve this access request?')) {
        approveBtn.disabled = true
        socket.send(
          JSON.stringify({
            command: 'approveAccessRequest',
            requestID: approveBtn.dataset.id,
          }),
        )
      }
    }

    const denyBtn = event.target.closest('.deny-btn')
    if (denyBtn) {
      const confirmText =
        denyBtn.textContent.trim() === 'Abort'
          ? 'Are you sure you want to abort your access request?'
          : 'Are you sure you want to deny this access request?'

      if (confirm(confirmText)) {
        denyBtn.disabled = true
        socket.send(
          JSON.stringify({
            command: 'denyAccessRequest',
            requestID: denyBtn.dataset.id,
          }),
        )
      }
    }
  })

  refreshServicesBtn.addEventListener('click', initializeCaFilters)
  refreshEaServicesBtn.addEventListener('click', initializeEaFilters)
  refreshAccessBtnDashboard.addEventListener(
    'click',
    fetchAndDisplayActiveAccesses,
  )
  refreshAccessBtnSvc.addEventListener('click', fetchAndDisplayActiveAccesses)
  refreshAccessBtnExt.addEventListener('click', fetchAndDisplayActiveAccesses)
  sourceNsFilter.addEventListener('change', (event) => {
    updateCaServiceDropdown(sourceSvcSelect, event.target.value)
  })
  targetNsFilter.addEventListener('change', (event) => {
    updateCaServiceDropdown(targetSvcSelect, event.target.value)
  })
  eaNsFilter.addEventListener('change', (event) => {
    updateCaServiceDropdown(eaServiceSelect, event.target.value)
  })

  showView('main-menu-view')
})
