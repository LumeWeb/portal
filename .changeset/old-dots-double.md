---
"@lumeweb/portal": minor
---

Major Features & Improvements:

Architecture & Core:
- Implemented unified request-based architecture for better request handling and processing
- Added support for cluster-aware configuration management with etcd
- Introduced new configuration structure with improved validation and management
- Implemented access control service and middleware with role-based permissions
- Added comprehensive event system

Storage & Upload Management:
- Revamped upload process with improved multi-part upload handling
- Added temporary upload functionality with S3 integration
- Implemented TUS protocol support

Database & Performance:
- Improved database operations with retry mechanisms
- Enhanced cron job reliability and error handling
- Optimized price tracking and database compatibility

Security & User Management:
- Added account verification system
- Implemented account deletion functionality
- Enhanced cookie handling and session management
- Improved password reset and email verification processes