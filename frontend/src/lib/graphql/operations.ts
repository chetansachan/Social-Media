/* ==========================================================================
   GraphQL operations — transcribed from schema.graphqls.
   --------------------------------------------------------------------------
   Field selections are explicit. Anything added to the schema must be added
   here in the same change.

   Four things this file deliberately does NOT do, because the schema does not:
     - `getAllPost` takes no limit/cursor and returns [Post!]! (a plain list).
     - `getFeed` returns FeedPost, which has no `updatedAt`.
     - `likePost` takes { postId, userId } and returns { isLiked, message }.
     - `User` has no `bio` and no `joinedAt`.
   ========================================================================== */

const USER_FIELDS = /* GraphQL */ `
  id
  username
  name
  email
  isPublic
  createdAt
  updatedAt
`;

const POST_FIELDS = /* GraphQL */ `
  id
  authorId
  content
  mediaUrl
  isPublic
  likeCount
  hasLiked
  createdAt
  updatedAt
  author {
    ${USER_FIELDS}
  }
`;

const FEED_POST_FIELDS = /* GraphQL */ `
  id
  authorId
  content
  mediaUrl
  isPublic
  likeCount
  hasLiked
  createdAt
  author {
    ${USER_FIELDS}
  }
`;

export const operations = {
  /* ---- Auth ------------------------------------------------------------ */
  login: /* GraphQL */ `
    mutation Login($input: LoginInput!) {
      Login(input: $input) {
        accessToken
        refressToken
        message
      }
    }
  `,

  logout: /* GraphQL */ `
    mutation Logout {
      logout
    }
  `,

  /* ---- Feed ------------------------------------------------------------ */
  feed: /* GraphQL */ `
    query GetFeed($limit: Int, $cursor: String) {
      getFeed(limit: $limit, cursor: $cursor) {
        hasMore
        nextCursor
        posts {
          ${FEED_POST_FIELDS}
        }
      }
    }
  `,

  /* ---- Posts ----------------------------------------------------------- */
  createPost: /* GraphQL */ `
    mutation CreatePost($input: CreatePostInput!) {
      CreatePost(input: $input) {
        ${POST_FIELDS}
      }
    }
  `,

  uploadSignature: /* GraphQL */ `
    query GetUploadSignature($resourceType: String, $folder: String) {
      getUploadSignature(resourceType: $resourceType, folder: $folder) {
        cloudName
        apiKey
        timestamp
        signature
        folder
        publicId
        uploadUrl
        expiresAt
      }
    }
  `,

  postById: /* GraphQL */ `
    query GetPostById($id: ID!) {
      getPostById(id: $id) {
        ${POST_FIELDS}
      }
    }
  `,

  /** No limit or cursor: the schema returns the whole list. */
  postsByAuthor: /* GraphQL */ `
    query GetAllPost($authorId: String!) {
      getAllPost(author_id: $authorId) {
        ${POST_FIELDS}
      }
    }
  `,

  /** The viewer's own liked posts; the gateway scopes this to the bearer. */
  likedPosts: /* GraphQL */ `
    query GetAllLikedPosts($userId: String!) {
      getAllLikedPosts(user_id: $userId) {
        ${POST_FIELDS}
      }
    }
  `,

  deletePost: /* GraphQL */ `
    mutation DeletePost($id: ID!) {
      deletePost(id: $id) {
        message
      }
    }
  `,

  /* ---- Likes ----------------------------------------------------------- */
  likePost: /* GraphQL */ `
    mutation LikePost($input: LikePostInput!) {
      likePost(input: $input) {
        isLiked
        message
      }
    }
  `,

  /* ---- Search ---------------------------------------------------------- */
  /**
   * One query string, three recall paths server-side: tsvector, ILIKE, and
   * hashtag array overlap (`p.hashtags && $2::text[]`). So `#golang` and
   * `golang` both work, and the client needs no separate tag endpoint.
   */
  searchPosts: /* GraphQL */ `
    query SearchPosts($query: String!, $limit: Int) {
      searchPosts(query: $query, limit: $limit) {
        ${POST_FIELDS}
      }
    }
  `,

  searchUsers: /* GraphQL */ `
    query SearchUsers($query: String!, $limit: Int) {
      searchUsers(query: $query, limit: $limit) {
        ${USER_FIELDS}
      }
    }
  `,

  /* ---- Users and follows ----------------------------------------------- */
  userById: /* GraphQL */ `
    query GetUserById($id: ID!) {
      getUserById(id: $id) {
        ${USER_FIELDS}
      }
    }
  `,

  userByUsername: /* GraphQL */ `
    query GetUserByUsername($username: String!) {
      getUserByUsername(username: $username) {
        ${USER_FIELDS}
      }
    }
  `,

  updateProfile: /* GraphQL */ `
    mutation UpdateUser($input: UpdateUserInput!) {
      UpdateUser(input: $input) {
        ${USER_FIELDS}
      }
    }
  `,

  followCounts: /* GraphQL */ `
    query GetFollowCounts($userId: String!) {
      getFollowCounts(userId: $userId) {
        followersCount
        followingCount
      }
    }
  `,

  followStatus: /* GraphQL */ `
    query GetFollowStatus($followerId: String!, $followingId: String!) {
      getFollowStatus(followerId: $followerId, followingId: $followingId) {
        isFollowing
      }
    }
  `,

  followUser: /* GraphQL */ `
    mutation FollowUser($input: FollowUserInput!) {
      followUser(input: $input) {
        message
      }
    }
  `,

  unfollowUser: /* GraphQL */ `
    mutation UnfollowUser($input: UnfollowUserInput!) {
      unfollowUser(input: $input) {
        message
      }
    }
  `,

  /* ---- Comments -------------------------------------------------------- */
  comments: /* GraphQL */ `
    query GetComments($postId: ID!, $parentId: ID, $limit: Int, $cursor: String) {
      getComments(postId: $postId, parentId: $parentId, limit: $limit, cursor: $cursor) {
        hasMore
        nextCursor
        comments {
          id
          postId
          authorId
          parentId
          content
          likeCount
          replyCount
          depth
          isDeleted
          hasLiked
          createdAt
          updatedAt
          author {
            id
            username
            name
          }
        }
      }
    }
  `,

  createComment: /* GraphQL */ `
    mutation CreateComment($input: CreateCommentInput!) {
      createComment(input: $input) {
        id
        postId
        authorId
        parentId
        content
        likeCount
        replyCount
        depth
        isDeleted
        hasLiked
        createdAt
        updatedAt
        author {
          id
          username
          name
        }
      }
    }
  `,

  likeComment: /* GraphQL */ `
    mutation LikeComment($input: LikeCommentInput!) {
      likeComment(input: $input) {
        isLiked
        message
      }
    }
  `,

  /* ---- Chat ------------------------------------------------------------ */
  conversations: /* GraphQL */ `
    query MyConversations($limit: Int, $cursor: String) {
      myConversations(limit: $limit, cursor: $cursor) {
        hasMore
        nextCursor
        totalUnread
        conversations {
          id
          type
          title
          createdBy
          unreadCount
          createdAt
          updatedAt
          participants {
            userId
            lastReadAt
            user {
              id
              username
              name
            }
          }
          lastMessage {
            id
            conversationId
            senderId
            content
            createdAt
            isDeleted
          }
        }
      }
    }
  `,

  messages: /* GraphQL */ `
    query GetMessages($conversationId: ID!, $limit: Int, $cursor: String) {
      getMessages(conversationId: $conversationId, limit: $limit, cursor: $cursor) {
        hasMore
        nextCursor
        messages {
          id
          conversationId
          senderId
          content
          createdAt
          isDeleted
          sender {
            id
            username
            name
          }
        }
      }
    }
  `,

  sendMessage: /* GraphQL */ `
    mutation SendMessage($input: SendMessageInput!) {
      sendMessage(input: $input) {
        id
        conversationId
        senderId
        content
        createdAt
        isDeleted
        sender {
          id
          username
          name
        }
      }
    }
  `,

  presence: /* GraphQL */ `
    query GetPresence($userIds: [ID!]!) {
      getPresence(userIds: $userIds)
    }
  `,

  markConversationRead: /* GraphQL */ `
    mutation MarkConversationRead($conversationId: ID!) {
      markConversationRead(conversationId: $conversationId)
    }
  `,

  createConversation: /* GraphQL */ `
    mutation CreateConversation($input: CreateConversationInput!) {
      createConversation(input: $input) {
        id
        type
        title
        createdBy
        unreadCount
        createdAt
        updatedAt
        participants {
          userId
          lastReadAt
          user {
            id
            username
            name
          }
        }
      }
    }
  `,

  /* ---- Notifications --------------------------------------------------- */
  notifications: /* GraphQL */ `
    query GetNotifications($limit: Int, $cursor: String, $filter: NotificationType) {
      getNotifications(limit: $limit, cursor: $cursor, filter: $filter) {
        hasMore
        nextCursor
        unreadCount
        notifications {
          id
          type
          entityId
          entityType
          title
          contentPreview
          actionUrl
          isRead
          createdAt
          totalActorsCount
          actors {
            id
            username
            displayName
            avatarUrl
          }
        }
      }
    }
  `,

  unreadCount: /* GraphQL */ `
    query GetUnreadNotificationCount {
      getUnreadNotificationCount
    }
  `,

  markAllNotificationsRead: /* GraphQL */ `
    mutation MarkAllNotificationsAsRead {
      markAllNotificationsAsRead
    }
  `,

  markNotificationsRead: /* GraphQL */ `
    mutation MarkNotificationAsRead($notificationIds: [ID!]!) {
      markNotificationAsRead(notificationIds: $notificationIds)
    }
  `,
} as const;
